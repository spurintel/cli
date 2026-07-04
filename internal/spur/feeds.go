package spur

import (
	"context"
	"fmt"
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

	segments := strings.Split(feedType, "/")
	for i, seg := range segments {
		segments[i] = url.PathEscape(seg)
	}
	endpoint := c.FeedsBase + "/v2/" + strings.Join(segments, "/") + "/latest"

	var out FeedMetadata
	if _, err := c.doJSON(ctx, endpoint, &out); err != nil {
		return FeedMetadata{}, err
	}
	out.Type = feedType
	return out, nil
}
