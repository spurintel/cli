package spur

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
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
// size is the CDN's Content-Length, or -1 when unknown.
//
// Unlike the JSON methods this does not buffer or size-limit the body: feeds are
// hundreds of megabytes and must stream. A slow or stuck transfer is bounded by
// ctx, not a client timeout. DownloadFeed does not check that feedType offers
// the requested format — callers that need a clear "not offered" error should
// consult GetFeedMetadata (its MMDB container) first.
func (c *Client) DownloadFeed(ctx context.Context, feedType, date string, format FeedFormat) (io.ReadCloser, int64, error) {
	feedType = strings.Trim(strings.TrimSpace(feedType), "/")
	if feedType == "" {
		return nil, 0, fmt.Errorf("feed type is required")
	}

	return c.doStream(ctx, c.FeedArtifactURL(feedType, date, format))
}

// doStream issues an authenticated GET for a large artifact on the download
// client (no overall timeout, Token stripped on cross-host redirects) and
// returns the raw body stream plus the Content-Length (-1 when unknown). The
// caller owns the body and must Close it. It is the shared request path for
// DownloadFeed and ExportFeed.
func (c *Client) doStream(ctx context.Context, endpoint string) (io.ReadCloser, int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Token", c.Token)
	req.Header.Set("User-Agent", userAgent())

	resp, err := c.downloadHTTPClient().Do(req)
	if err != nil {
		return nil, 0, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain and close so the connection can be reused; the body is an
		// upstream error page we deliberately do not surface.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseBodyBytes))
		_ = resp.Body.Close()
		return nil, 0, newAPIError(resp.StatusCode)
	}

	return resp.Body, resp.ContentLength, nil
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
