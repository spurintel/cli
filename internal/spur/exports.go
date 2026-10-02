package spur

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ExportsBaseURL is the root of the Spur Exports API. It is a distinct host from
// the Context API (BaseURL) and the Data Feeds API (FeedsBaseURL): it serves
// dynamically filtered and reformatted subsets of the feeds, authenticated with
// the same Token header.
const ExportsBaseURL = "https://exports.spur.us"

// ExportFeed describes one feed family served by the Exports API and which
// filters that family accepts. The Exports API rejects an unknown query
// parameter with 400, and each family accepts a different subset (for example
// callbackProxies is residential-only, diff is anon/ztna-only, and dch/ipgeo
// take neither tags nor a limit). Gating the flags client-side turns those into
// clear up-front errors instead of a round-trip that spends a rate-limited
// request only to fail.
//
// The catalog holds the base (IPv4) feeds; the -ipv6 variant of a family that
// has one is addressed by appending "-ipv6" to the slug (see ResolveExportFeed),
// and shares the base family's capabilities.
type ExportFeed struct {
	// Name is the canonical feed slug: the `spur export <name>` argument and
	// the /v1/feeds/<name> path segment.
	Name string `json:"name" jsonschema:"Canonical export feed slug, e.g. anonymous or anonymous-residential."`
	// Description is a one-line summary of the feed's contents.
	Description string `json:"description" jsonschema:"Short human-readable summary of the feed's contents."`
	// HasIPv6 reports whether the family has a -ipv6 variant.
	HasIPv6 bool `json:"has_ipv6" jsonschema:"Whether the feed has an IPv6 variant addressed with --ipv6."`

	// Capability gates. Each reports whether the family accepts the
	// correspondingly named filter; asns, fields, output, and the CSV header
	// are accepted by every family and so have no gate.
	Tags            bool `json:"-"`
	CallbackProxies bool `json:"-"`
	Ccs             bool `json:"-"`
	Limit           bool `json:"-"`
	Attributed      bool `json:"-"`
	Diff            bool `json:"-"`
}

// ExportFeeds is the catalog of Exports API feed families the CLI knows about,
// in the order the Exports documentation lists them. It drives `spur export`
// argument validation and, per family, which flags are accepted.
var ExportFeeds = []ExportFeed{
	{
		Name: "anonymous", Description: "Anonymizing proxy and VPN IPs", HasIPv6: true,
		Tags: true, Ccs: true, Limit: true, Attributed: true, Diff: true,
	},
	{
		Name: "anonymous-residential", Description: "Anonymizing plus residential proxy IPs", HasIPv6: true,
		Tags: true, CallbackProxies: true, Ccs: true, Limit: true, Attributed: true,
	},
	{
		Name: "ztna", Description: "Zero Trust Network Access (ZTNA) IPs", HasIPv6: true,
		Tags: true, Ccs: true, Limit: true, Attributed: true, Diff: true,
	},
	{
		Name: "dch", Description: "Datacenter hosting (DCH) ranges",
	},
	{
		Name: "ipgeo", Description: "IP geolocation", Ccs: true,
	},
}

// LookupExportFeed returns the catalog entry with the given base slug and
// whether it exists. Slugs are matched exactly and are lowercase by convention.
func LookupExportFeed(name string) (ExportFeed, bool) {
	for _, f := range ExportFeeds {
		if f.Name == name {
			return f, true
		}
	}
	return ExportFeed{}, false
}

// ResolveExportFeed turns the user's feed argument plus the --ipv6 modifier into
// a catalog family and the concrete URL slug. The modifier is convenience sugar:
// `anonymous --ipv6` resolves to the anonymous-ipv6 slug, but the full "-ipv6"
// slug may also be passed directly. A family that has no IPv6 variant, or an
// unknown family, is rejected before any network call. Capabilities always come
// from the base family; the returned slug is what the request path uses.
func ResolveExportFeed(arg string, ipv6 bool) (ExportFeed, string, error) {
	base := strings.ToLower(strings.TrimSpace(arg))
	if strings.HasSuffix(base, "-ipv6") {
		base = strings.TrimSuffix(base, "-ipv6")
		ipv6 = true
	}

	feed, ok := LookupExportFeed(base)
	if !ok {
		return ExportFeed{}, "", fmt.Errorf("unknown export feed %q; run 'spur export --help' for the supported feeds", arg)
	}
	if ipv6 && !feed.HasIPv6 {
		return ExportFeed{}, "", fmt.Errorf("the %q feed has no IPv6 variant; drop --ipv6", feed.Name)
	}

	slug := feed.Name
	if ipv6 {
		slug += "-ipv6"
	}
	return feed, slug, nil
}

// ValidExportFormats is the set of output formats the Exports API produces.
var ValidExportFormats = []string{"csv", "json", "mmdb"}

func isValidExportFormat(f string) bool {
	for _, v := range ValidExportFormats {
		if f == v {
			return true
		}
	}
	return false
}

// ExportOptions carries the filter and format selections for an export request,
// as gathered from the command-line flags. It is the input to BuildExportQuery.
// Zero values mean "not set": an empty slice or string, a zero Limit, and a nil
// Attributed are all omitted from the query so the server applies its default.
type ExportOptions struct {
	// Format is the output format: "csv", "json", or "mmdb".
	Format string
	// Fields selects and orders the output columns.
	Fields []string
	// Tags filters to IPs carrying any of these service tags.
	Tags []string
	// Ccs filters to these ISO country codes.
	Ccs []string
	// Asns filters to these autonomous-system numbers.
	Asns []int
	// CallbackProxies filters to these callback-proxy providers (residential).
	CallbackProxies []string
	// Attributed, when non-nil, includes (true) or excludes (false) IPs that
	// are not attributed to a specific service. Nil leaves the server default.
	Attributed *bool
	// Diff requests only the changes versus the previous day.
	Diff bool
	// DiffDate requests the diff against a specific date (YYYY-MM-DD); it
	// implies Diff.
	DiffDate string
	// Limit caps the number of rows returned. Zero means no limit.
	Limit int64
	// CSVHeader adds a header row to CSV output.
	CSVHeader bool
}

// BuildExportQuery maps validated ExportOptions onto the Exports API query
// parameters for a given feed family, or returns an error naming the first
// filter that the family does not accept. The parameter names mirror the API
// exactly (output, fields, tags, ccs, asns, callbackProxies, attributed, diff,
// diffDate, limit, includeCSVHeader). This flag-to-parameter mapping is the
// logic under test; the command layer is a thin adapter over it.
func BuildExportQuery(feed ExportFeed, o ExportOptions) (url.Values, error) {
	q := url.Values{}

	if o.Format != "" {
		if !isValidExportFormat(o.Format) {
			return nil, fmt.Errorf("invalid --format %q; must be one of: %s", o.Format, strings.Join(ValidExportFormats, ", "))
		}
		q.Set("output", o.Format)
	}

	if len(o.Fields) > 0 {
		q.Set("fields", strings.Join(o.Fields, ","))
	}

	if len(o.Tags) > 0 {
		if !feed.Tags {
			return nil, unsupportedExportFlag(feed, "--tags")
		}
		q.Set("tags", strings.Join(o.Tags, ","))
	}

	if len(o.Ccs) > 0 {
		if !feed.Ccs {
			return nil, unsupportedExportFlag(feed, "--ccs")
		}
		q.Set("ccs", strings.ToUpper(strings.Join(o.Ccs, ",")))
	}

	if len(o.Asns) > 0 {
		strs := make([]string, len(o.Asns))
		for i, a := range o.Asns {
			strs[i] = strconv.Itoa(a)
		}
		q.Set("asns", strings.Join(strs, ","))
	}

	if len(o.CallbackProxies) > 0 {
		if !feed.CallbackProxies {
			return nil, unsupportedExportFlag(feed, "--callback-proxies")
		}
		q.Set("callbackProxies", strings.Join(o.CallbackProxies, ","))
	}

	if o.Attributed != nil {
		if !feed.Attributed {
			return nil, unsupportedExportFlag(feed, "--attributed")
		}
		q.Set("attributed", strconv.FormatBool(*o.Attributed))
	}

	if o.Diff || o.DiffDate != "" {
		if !feed.Diff {
			return nil, unsupportedExportFlag(feed, "--diff/--diff-date")
		}
		if o.DiffDate != "" {
			if _, err := time.Parse("2006-01-02", o.DiffDate); err != nil {
				return nil, fmt.Errorf("invalid --diff-date %q; want YYYY-MM-DD", o.DiffDate)
			}
			// The server infers diff=true from diffDate, so sending the date
			// alone is sufficient and unambiguous.
			q.Set("diffDate", o.DiffDate)
		} else {
			q.Set("diff", "true")
		}
	}

	if o.Limit > 0 {
		if !feed.Limit {
			return nil, unsupportedExportFlag(feed, "--limit")
		}
		q.Set("limit", strconv.FormatInt(o.Limit, 10))
	}

	if o.CSVHeader {
		q.Set("includeCSVHeader", "true")
	}

	return q, nil
}

// unsupportedExportFlag builds the error for a filter the feed does not accept,
// listing the feeds that do so the user can see where the flag belongs.
func unsupportedExportFlag(feed ExportFeed, flag string) error {
	var accepts []string
	for _, f := range ExportFeeds {
		ok := false
		switch flag {
		case "--tags":
			ok = f.Tags
		case "--ccs":
			ok = f.Ccs
		case "--callback-proxies":
			ok = f.CallbackProxies
		case "--attributed":
			ok = f.Attributed
		case "--diff/--diff-date":
			ok = f.Diff
		case "--limit":
			ok = f.Limit
		}
		if ok {
			accepts = append(accepts, f.Name)
		}
	}
	sort.Strings(accepts)
	if len(accepts) == 0 {
		return fmt.Errorf("the %q feed does not accept %s", feed.Name, flag)
	}
	return fmt.Errorf("the %q feed does not accept %s (accepted by: %s)", feed.Name, flag, strings.Join(accepts, ", "))
}

// ExportFeed streams a filtered export from the Exports host. slug is the
// resolved feed slug (including any -ipv6 suffix) and q is the query built by
// BuildExportQuery. The response is the raw artifact stream — CSV, newline-JSON,
// or a MaxMind DB, per the output parameter — which the caller owns and must
// Close.
//
// Like DownloadFeed this does not buffer or size-limit the body: a full export
// can be large and must stream. A slow or stuck transfer is bounded by ctx, not
// a client timeout. A non-2xx status is drained (so the connection can be
// reused) and mapped to an APIError; a 429 becomes ErrThrottled so the command
// layer can surface a rate-limit message.
func (c *Client) ExportFeed(ctx context.Context, slug string, q url.Values) (io.ReadCloser, int64, error) {
	slug = strings.Trim(strings.TrimSpace(slug), "/")
	if slug == "" {
		return nil, 0, fmt.Errorf("export feed is required")
	}

	endpoint := c.ExportsBase + "/v1/feeds/" + url.PathEscape(slug)
	if enc := q.Encode(); enc != "" {
		endpoint += "?" + enc
	}
	body, integ, err := c.doStream(ctx, endpoint)
	return body, integ.Size, err
}
