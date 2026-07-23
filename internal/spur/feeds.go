package spur

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// FeedsBaseURL is the root of the Spur Data Feeds API. It is a distinct host
// from the Context API (BaseURL): bulk feed files and their metadata documents
// live here, authenticated with the same Token header.
const FeedsBaseURL = "https://feeds.spur.us"

// FeedType is one entry in the Spur data-feed catalog: the identifier a user
// passes on the command line (and the path segment[s] on the feeds host) plus a
// short human-readable summary for `spur feed list`.
//
// Realtime and IPv6 variants are distinct feed types with their own slugs; the
// realtime slug is a nested path (anonymous-residential/realtime), addressed the
// same way in the URL.
type FeedType struct {
	// Name is the canonical feed identifier, used verbatim both as the
	// `spur feed status <name>` argument and as the /v2/<name>/latest path.
	Name string `json:"name" jsonschema:"Canonical feed identifier, e.g. anonymous or anonymous-residential/realtime."`
	// Description is a one-line summary of what the feed contains.
	Description string `json:"description" jsonschema:"Short human-readable summary of the feed's contents."`
}

// FeedTypes is the catalog of feed types the CLI knows about, in the order the
// Data Feeds documentation lists them. It drives `spur feed list` and validates
// the `spur feed status <type>` argument before any network call.
var FeedTypes = []FeedType{
	{Name: "anonymous", Description: "Anonymizing proxy and VPN IPs (IPv4)"},
	{Name: "anonymous-ipv6", Description: "Anonymizing proxy and VPN IPs (IPv6)"},
	{Name: "anonymous-residential", Description: "Anonymizing plus residential proxy IPs (IPv4)"},
	{Name: "anonymous-residential-ipv6", Description: "Anonymizing plus residential proxy IPs (IPv6)"},
	{Name: "anonymous-residential/realtime", Description: "Anonymizing plus residential, refreshed intraday"},
	{Name: "service-metrics", Description: "Aggregate metrics per tracked service"},
	{Name: "dch", Description: "Datacenter hosting (DCH) ranges"},
	{Name: "ipgeo", Description: "IP geolocation (also offered as MMDB)"},
	{Name: "ipsummary", Description: "Per-IP summary of Spur attributes"},
}

// FeedFormat selects which artifact of a feed to download. Feeds are always
// published as newline-delimited-JSON gzip; some (e.g. ipgeo, anonymous) are
// additionally published as a MaxMind DB. Whether a given feed offers MMDB is
// authoritative in its metadata document (FeedMetadata.MMDB), not a static list.
type FeedFormat int

const (
	// FeedJSONGzip is the newline-delimited-JSON gzip artifact (.json.gz).
	FeedJSONGzip FeedFormat = iota
	// FeedMMDB is the MaxMind DB artifact (.mmdb). Not gzipped.
	FeedMMDB
)

// LookupFeedType returns the catalog entry with the given name and whether it
// exists. Feed names are matched exactly; they are lowercase by convention.
func LookupFeedType(name string) (FeedType, bool) {
	for _, ft := range FeedTypes {
		if ft.Name == name {
			return ft, true
		}
	}
	return FeedType{}, false
}

// escapeFeedType path-escapes each segment of a (possibly nested) feed type so a
// nested slug like anonymous-residential/realtime keeps its slash while every
// segment is individually escaped.
func escapeFeedType(feedType string) string {
	segments := strings.Split(strings.Trim(strings.TrimSpace(feedType), "/"), "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	return strings.Join(segments, "/")
}

// FeedArtifactURL returns the absolute feeds-host URL of a feed's data artifact.
// With an empty date it points at the latest release (`.../latest.json.gz` or
// `.../latest.mmdb`); otherwise at the historical file for that YYYYMMDD
// (`.../<date>/feed.json.gz` or `.../<date>/feed.mmdb`). The URL is authenticated
// with the Token header and 302-redirects to the CDN. It is exposed so callers
// that must surface a download pointer without streaming the bytes (e.g. the MCP
// feed tool) share DownloadFeed's exact path construction rather than duplicating
// it.
func (c *Client) FeedArtifactURL(feedType, date string, format FeedFormat) string {
	latestName, dailyName := "latest.json.gz", "feed.json.gz"
	if format == FeedMMDB {
		latestName, dailyName = "latest.mmdb", "feed.mmdb"
	}
	endpoint := c.FeedsBase + "/v2/" + escapeFeedType(feedType)
	if date == "" {
		return endpoint + "/" + latestName
	}
	return endpoint + "/" + url.PathEscape(date) + "/" + dailyName
}

// GetFeedMetadata calls GET /v2/{feedType}/latest on the feeds host and returns
// the parsed Feed Metadata document with its Type field populated.
//
// feedType may contain a nested path segment (anonymous-residential/realtime);
// each segment is path-escaped independently so the slash is preserved. The
// method does not validate feedType against the catalog — that is the command
// layer's job, so the client stays usable if the server catalog drifts.
func (c *Client) GetFeedMetadata(ctx context.Context, feedType string) (FeedMetadata, error) {
	feedType = strings.Trim(strings.TrimSpace(feedType), "/")
	if feedType == "" {
		return FeedMetadata{}, fmt.Errorf("feed type is required")
	}

	endpoint := c.FeedsBase + "/v2/" + escapeFeedType(feedType) + "/latest"

	var out FeedMetadata
	if _, err := c.doJSON(ctx, endpoint, &out); err != nil {
		return FeedMetadata{}, err
	}
	out.Type = feedType
	return out, nil
}

// DownloadFeed streams a feed's data file from the feeds host in the requested
// format. When date is empty it fetches the current release (`.../latest.json.gz`
// or `.../latest.mmdb`); otherwise it fetches the historical file for that
// YYYYMMDD (`.../<date>/feed.json.gz` or `.../<date>/feed.mmdb`).
//
// feedType may contain a nested segment (anonymous-residential/realtime); each
// segment is path-escaped independently so the slash is preserved. The initial
// request carries the Token header; the feeds host answers with a redirect to
// the CDN (storage.googleapis.com), which is followed automatically — but the
// Token is stripped before leaving the feeds host so it is never handed to the
// third-party CDN (Go's default policy strips Authorization/Cookie but not a
// custom header). The returned body is the raw artifact stream (gzip for JSON,
// an uncompressed MaxMind DB for MMDB); the caller owns it and must Close it.
// integ is the CDN's size/CRC32C/MD5, per integrityFromResponse.
//
// Unlike the JSON methods this does not buffer or size-limit the body: feeds are
// hundreds of megabytes and must stream. A slow or stuck transfer is bounded by
// ctx, not a client timeout. DownloadFeed does not check that feedType offers
// the requested format — callers that need a clear "not offered" error should
// consult GetFeedMetadata (its MMDB container) first.
func (c *Client) DownloadFeed(ctx context.Context, feedType, date string, format FeedFormat) (io.ReadCloser, FeedIntegrity, error) {
	feedType = strings.Trim(strings.TrimSpace(feedType), "/")
	if feedType == "" {
		return nil, FeedIntegrity{}, fmt.Errorf("feed type is required")
	}

	return c.doStream(ctx, c.FeedArtifactURL(feedType, date, format))
}

// parseGoogHash extracts the crc32c and md5 digests from the raw values of an
// X-Goog-Hash response header. Each element of values may itself be a
// comma-joined list (crc32c=..,md5=..), which is how GCS emits the header when
// both digests are present; multiple separate header lines are also accepted.
// Unknown keys are ignored; a missing digest returns "" for that return value.
func parseGoogHash(values []string) (crc32c, md5 string) {
	for _, v := range values {
		for _, part := range strings.Split(v, ",") {
			part = strings.TrimSpace(part)
			switch {
			case strings.HasPrefix(part, "crc32c="):
				crc32c = strings.TrimSpace(strings.TrimPrefix(part, "crc32c="))
			case strings.HasPrefix(part, "md5="):
				md5 = strings.TrimSpace(strings.TrimPrefix(part, "md5="))
			}
		}
	}
	return crc32c, md5
}

// integrityFromResponse derives a FeedIntegrity from a download response's
// headers. Size prefers the total from a Content-Range header (present on
// partial-content responses, where Content-Length reflects only the returned
// range) and falls back to Content-Length, else -1 when neither is available.
// CRC32C/MD5 come from X-Goog-Hash via parseGoogHash.
func integrityFromResponse(resp *http.Response) FeedIntegrity {
	crc32c, md5 := parseGoogHash(resp.Header.Values("X-Goog-Hash"))
	return FeedIntegrity{
		Size:   sizeFromResponse(resp),
		CRC32C: crc32c,
		MD5:    md5,
	}
}

// sizeFromResponse resolves the object's total size: the Content-Range total
// when present and parseable, else resp.ContentLength when known, else the raw
// Content-Length header, else -1.
func sizeFromResponse(resp *http.Response) int64 {
	if total, ok := contentRangeTotal(resp.Header.Get("Content-Range")); ok {
		return total
	}
	if resp.ContentLength >= 0 {
		return resp.ContentLength
	}
	if cl := resp.Header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil {
			return n
		}
	}
	return -1
}

// contentRangeTotal parses the total-length component of a Content-Range
// header value (bytes 0-0/12345 -> 12345, true). A missing or non-numeric
// total (e.g. the "*" GCS uses when the size is unknown) reports ok=false.
func contentRangeTotal(headerValue string) (int64, bool) {
	idx := strings.LastIndex(headerValue, "/")
	if idx == -1 || idx == len(headerValue)-1 {
		return 0, false
	}
	total, err := strconv.ParseInt(headerValue[idx+1:], 10, 64)
	if err != nil {
		return 0, false
	}
	return total, true
}

// doStream issues an authenticated GET for a large artifact on the download
// client (no overall timeout, Token stripped on cross-host redirects) and
// returns the raw body stream plus its FeedIntegrity (size, CRC32C, MD5) per
// integrityFromResponse. The caller owns the body and must Close it. It is the
// shared request path for DownloadFeed and ExportFeed.
func (c *Client) doStream(ctx context.Context, endpoint string) (io.ReadCloser, FeedIntegrity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, FeedIntegrity{}, err
	}
	req.Header.Set("Token", c.Token)
	req.Header.Set("User-Agent", c.userAgent())

	resp, err := c.downloadHTTPClient().Do(req)
	if err != nil {
		return nil, FeedIntegrity{}, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain and close so the connection can be reused; the body is an
		// upstream error page we deliberately do not surface.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
		_ = resp.Body.Close()
		return nil, FeedIntegrity{}, newAPIError(resp.StatusCode)
	}

	return resp.Body, integrityFromResponse(resp), nil
}

// downloadHTTPClient returns an HTTP client tuned for large streaming downloads:
// no overall timeout (transfers are bounded by the request context, not a
// deadline that would kill a legitimately long download) and a redirect policy
// that drops the Token header once the target host changes. It reuses the base
// client's Transport so any configured proxy/TLS settings still apply.
func (c *Client) downloadHTTPClient() *http.Client {
	var transport http.RoundTripper
	if c.HTTP != nil {
		transport = c.HTTP.Transport
	}
	return &http.Client{
		Transport:     transport,
		CheckRedirect: stripTokenOnHostChange,
	}
}

// stripTokenOnHostChange follows redirects but removes the Token header on any
// hop that leaves the original host — so a pre-signed CDN URL never receives
// the Spur credential — or that downgrades from HTTPS to plain HTTP, so the
// token is never re-sent in cleartext. It also enforces Go's default
// 10-redirect ceiling. It is the CheckRedirect policy for every client here:
// Go's default only strips Authorization/Cookie cross-host, not custom headers.
func stripTokenOnHostChange(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	if len(via) == 0 {
		return nil
	}
	crossHost := req.URL.Host != via[0].URL.Host
	downgraded := via[0].URL.Scheme == "https" && req.URL.Scheme != "https"
	if crossHost || downgraded {
		req.Header.Del("Token")
	}
	return nil
}
