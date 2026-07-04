package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spurintel/cli/internal/spur"
)

// ListFeedsInput is intentionally empty — the catalog takes no arguments.
type ListFeedsInput struct{}

// ListFeedsOutput is the static Spur data-feed catalog.
type ListFeedsOutput struct {
	Feeds []spur.FeedType `json:"feeds" jsonschema:"The supported Spur data-feed types, including IPv6 and realtime variants. Each name is what you pass to feed_status and feed_download."`
}

// registerListFeeds wires the list_feeds tool. Like `spur feed list` it returns
// the built-in catalog (spur.FeedTypes) with no network call, so the AI-facing
// enumeration stays in lockstep with the terminal one.
func registerListFeeds(s *mcp.Server) {
	mcp.AddTool(s, &mcp.Tool{
		Name:  "list_feeds",
		Title: "List Spur data feeds",
		Description: "Lists the Spur data-feed catalog (bulk intelligence delivered from " +
			"feeds.spur.us): each feed's canonical type identifier and a one-line " +
			"description, including the IPv6 and realtime variants. Use the returned " +
			"name with feed_status (current metadata) or feed_download (download " +
			"pointer). No network call — the catalog is static.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ListFeedsInput) (*mcp.CallToolResult, ListFeedsOutput, error) {
		return nil, ListFeedsOutput{Feeds: spur.FeedTypes}, nil
	})
}

// FeedStatusInput is the argument object for feed_status.
type FeedStatusInput struct {
	Type string `json:"type" jsonschema:"Feed type identifier from list_feeds, e.g. anonymous or anonymous-residential/realtime."`
}

// FeedStatusOutput wraps the feed's metadata document.
type FeedStatusOutput struct {
	Metadata spur.FeedMetadata `json:"metadata" jsonschema:"The feed's current metadata: date, generation/availability timestamps, relative location, and filesize for the JSON (and, where offered, MMDB) artifact."`
}

// registerFeedStatus wires the feed_status tool, backed by the same
// client.GetFeedMetadata that `spur feed status` calls. The feed type is
// validated against the catalog first (matching the command) so an unknown type
// is a clear error rather than a 404.
func registerFeedStatus(s *mcp.Server, c *spur.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:  "feed_status",
		Title: "Show a Spur feed's metadata",
		Description: "Fetches the current metadata for a Spur data feed: the file's date, " +
			"when it was generated and made available, its relative location, and its " +
			"size (for the JSON artifact and, where offered, the MMDB). Use list_feeds " +
			"for the valid type identifiers. Cheap metadata — no bulk data is returned.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in FeedStatusInput) (*mcp.CallToolResult, FeedStatusOutput, error) {
		feedType, ok := spur.LookupFeedType(strings.TrimSpace(in.Type))
		if !ok {
			return toolError(fmt.Errorf("unknown feed type %q", in.Type), "Call list_feeds for the supported feed types."), FeedStatusOutput{}, nil
		}
		md, err := c.GetFeedMetadata(ctx, feedType.Name)
		if err != nil {
			return toolError(err, statusHint(err)), FeedStatusOutput{}, nil
		}
		return nil, FeedStatusOutput{Metadata: md}, nil
	})
}

// FeedDownloadInput is the argument object for feed_download.
type FeedDownloadInput struct {
	Type   string `json:"type" jsonschema:"Feed type identifier from list_feeds, e.g. anonymous or anonymous-residential/realtime."`
	Format string `json:"format,omitempty" jsonschema:"Artifact to describe: json (newline-delimited JSON gzip, the default) or mmdb (MaxMind DB, only where the feed offers it)."`
	Date   string `json:"date,omitempty" jsonschema:"Optional historical release date (YYYYMMDD). Omit for the latest release."`
}

// FeedDownloadOutput is a download pointer for a feed's data artifact: enough
// metadata to decide whether to fetch it, plus the URL and command that do.
type FeedDownloadOutput struct {
	Type        string        `json:"type" jsonschema:"The feed type this pointer describes."`
	Format      string        `json:"format" jsonschema:"The artifact format described: json or mmdb."`
	File        spur.FeedFile `json:"file" jsonschema:"Metadata for the selected artifact: date, relative location, generation/availability timestamps, and filesize in bytes."`
	DownloadURL string        `json:"download_url" jsonschema:"Absolute feeds-host URL of the artifact. It is authenticated with the Token header and 302-redirects to the CDN; it is not returned inline here."`
	Command     string        `json:"command" jsonschema:"The spur CLI command that downloads this artifact."`
	Note        string        `json:"note" jsonschema:"Why the bytes are not returned inline."`
}

// registerFeedDownload wires the feed_download tool. Feeds are hundreds of
// megabytes, so returning the bytes over stdio MCP is a non-starter; this is a
// metadata-first tool. It calls the same client.GetFeedMetadata that the
// `spur feed download` command consults (for its artifact/format checks) and
// surfaces the artifact's size, date, and location plus the URL and CLI command
// that fetch it — never the payload itself.
func registerFeedDownload(s *mcp.Server, c *spur.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:  "feed_download",
		Title: "Get a Spur feed download pointer",
		Description: "Returns a metadata-first download pointer for a Spur data feed: the " +
			"artifact's date, size, and location, plus the authenticated feeds URL and " +
			"the `spur feed download` command that fetch it. Feeds are large bulk files " +
			"and are NOT returned inline over MCP — use the URL or command to download. " +
			"Use list_feeds for the valid type identifiers; 'format' selects json " +
			"(default) or mmdb (only where offered).",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in FeedDownloadInput) (*mcp.CallToolResult, FeedDownloadOutput, error) {
		feedType, ok := spur.LookupFeedType(strings.TrimSpace(in.Type))
		if !ok {
			return toolError(fmt.Errorf("unknown feed type %q", in.Type), "Call list_feeds for the supported feed types."), FeedDownloadOutput{}, nil
		}

		format := spur.FeedJSONGzip
		formatName := "json"
		switch strings.ToLower(strings.TrimSpace(in.Format)) {
		case "", "json":
			// default
		case "mmdb":
			format, formatName = spur.FeedMMDB, "mmdb"
		default:
			return toolError(fmt.Errorf("invalid format %q", in.Format), "Use json (default) or mmdb."), FeedDownloadOutput{}, nil
		}

		date := strings.TrimSpace(in.Date)
		if date != "" && strings.HasSuffix(feedType.Name, "/realtime") {
			return toolError(fmt.Errorf("the realtime feed does not support a historical date"), "Omit 'date' for the realtime feed."), FeedDownloadOutput{}, nil
		}

		md, err := c.GetFeedMetadata(ctx, feedType.Name)
		if err != nil {
			return toolError(err, statusHint(err)), FeedDownloadOutput{}, nil
		}

		file := md.JSON
		command := "spur feed download " + feedType.Name
		if format == spur.FeedMMDB {
			if md.MMDB == nil {
				return toolError(fmt.Errorf("the %q feed is not offered in MMDB format", feedType.Name), "Request format json instead."), FeedDownloadOutput{}, nil
			}
			file = *md.MMDB
			command += " --mmdb"
		}
		if date != "" {
			command += " --date " + date
		}

		return nil, FeedDownloadOutput{
			Type:        feedType.Name,
			Format:      formatName,
			File:        file,
			DownloadURL: c.FeedArtifactURL(feedType.Name, date, format),
			Command:     command,
			Note:        "Metadata only. Spur feeds are large bulk files and are not returned inline over MCP; fetch the artifact with the command or a GET to download_url carrying the Token header.",
		}, nil
	})
}
