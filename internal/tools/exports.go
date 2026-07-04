package tools

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/spurintel/cli/internal/spur"
)

// exportSampleDefault and exportSampleMax bound the number of rows the export
// tool returns inline. A full export streams unbounded from the Exports API;
// over MCP we return a sample only, so an AI client can inspect the shape and
// content without a multi-megabyte dump on the transport.
const (
	exportSampleDefault = 50
	exportSampleMax     = 1000
)

// ExportInput is the argument object for the export tool. It surfaces the same
// filters as `spur export`; which ones a given feed accepts is enforced by
// spur.BuildExportQuery, exactly as the command does.
type ExportInput struct {
	Feed            string   `json:"feed" jsonschema:"Export feed: anonymous, anonymous-residential, ztna, dch, or ipgeo."`
	IPv6            bool     `json:"ipv6,omitempty" jsonschema:"Select the feed's IPv6 variant (anonymous, anonymous-residential, ztna)."`
	Format          string   `json:"format,omitempty" jsonschema:"Sample format: csv (default) or json. mmdb is binary and not returned over MCP."`
	Fields          []string `json:"fields,omitempty" jsonschema:"Columns to include and their order (feed-specific; default is the feed's full set)."`
	Tags            []string `json:"tags,omitempty" jsonschema:"Filter to IPs carrying any of these service tags (not accepted by dch/ipgeo)."`
	Ccs             []string `json:"ccs,omitempty" jsonschema:"Filter to these ISO country codes (not accepted by dch)."`
	Asns            []int    `json:"asns,omitempty" jsonschema:"Filter to these autonomous-system numbers."`
	CallbackProxies []string `json:"callback_proxies,omitempty" jsonschema:"Filter to these callback-proxy providers (anonymous-residential only)."`
	Attributed      *bool    `json:"attributed,omitempty" jsonschema:"Include only IPs attributed to a service (true) or include unattributed too (false). Omit for the server default."`
	Diff            bool     `json:"diff,omitempty" jsonschema:"Return only changes versus the previous day (anonymous, ztna)."`
	DiffDate        string   `json:"diff_date,omitempty" jsonschema:"Return the diff against this date (YYYY-MM-DD; implies diff)."`
	CSVHeader       bool     `json:"csv_header,omitempty" jsonschema:"Prepend a header row to CSV output."`
	MaxRows         int      `json:"max_rows,omitempty" jsonschema:"Maximum rows to return in the sample (default 50, hard cap 1000)."`
}

// ExportOutput is a bounded sample of an export plus enough context to fetch the
// full stream out-of-band.
type ExportOutput struct {
	Feed      string   `json:"feed" jsonschema:"The resolved export feed family."`
	Slug      string   `json:"slug" jsonschema:"The concrete feed slug queried (includes any -ipv6 suffix)."`
	Format    string   `json:"format" jsonschema:"The sample format: csv or json."`
	Rows      []string `json:"rows" jsonschema:"A bounded sample of raw export rows (CSV lines, or one JSON object per line), capped at max_rows."`
	RowCount  int      `json:"row_count" jsonschema:"Number of rows in this sample."`
	Truncated bool     `json:"truncated" jsonschema:"True when the export had more rows than the sample cap; run the command for the full result."`
	Command   string   `json:"command" jsonschema:"The spur CLI command that streams the full, unbounded export."`
}

// registerExport wires the export tool. It calls the same client.ExportFeed the
// `spur export` command uses and builds its query with the same
// spur.BuildExportQuery, but reads only a bounded number of rows off the stream
// (closing it early) so a large export never floods the MCP transport.
func registerExport(s *mcp.Server, c *spur.Client) {
	mcp.AddTool(s, &mcp.Tool{
		Name:  "export",
		Title: "Sample a filtered Spur export",
		Description: "Returns a BOUNDED sample of a dynamically filtered Spur export (from the " +
			"Exports API). Feeds: anonymous, anonymous-residential, ztna, dch, ipgeo " +
			"(add ipv6 for the IPv6 variant where offered). Filters mirror `spur export` " +
			"(fields, tags, ccs, asns, callback_proxies, attributed, diff, diff_date); a " +
			"filter a feed does not accept is rejected up front. Output is capped at " +
			"max_rows (default 50, max 1000) — this is a sample, not a bulk dump; run the " +
			"returned command for the full stream. mmdb output is not supported here.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in ExportInput) (*mcp.CallToolResult, ExportOutput, error) {
		feed, slug, err := spur.ResolveExportFeed(in.Feed, in.IPv6)
		if err != nil {
			return toolError(err, "Call the tool with feed set to one of: anonymous, anonymous-residential, ztna, dch, ipgeo."), ExportOutput{}, nil
		}

		format := strings.ToLower(strings.TrimSpace(in.Format))
		if format == "" {
			format = "csv"
		}
		if format == "mmdb" {
			return toolError(fmt.Errorf("mmdb output is binary and not returned over MCP"), "Use format csv or json, or run 'spur export --format mmdb -o file' for the binary artifact."), ExportOutput{}, nil
		}

		sampleCap := in.MaxRows
		if sampleCap <= 0 {
			sampleCap = exportSampleDefault
		}
		if sampleCap > exportSampleMax {
			sampleCap = exportSampleMax
		}

		opts := spur.ExportOptions{
			Format:          format,
			Fields:          in.Fields,
			Tags:            in.Tags,
			Ccs:             in.Ccs,
			Asns:            in.Asns,
			CallbackProxies: in.CallbackProxies,
			Attributed:      in.Attributed,
			Diff:            in.Diff,
			DiffDate:        in.DiffDate,
			CSVHeader:       in.CSVHeader,
		}
		// When the feed supports a server-side limit, cap it too so the API does
		// less work than we would otherwise discard. Feeds that reject --limit
		// (dch, ipgeo) are bounded purely by reading a fixed number of rows.
		if feed.Limit {
			opts.Limit = int64(sampleCap + 1)
		}

		q, err := spur.BuildExportQuery(feed, opts)
		if err != nil {
			return toolError(err, ""), ExportOutput{}, nil
		}

		body, _, err := c.ExportFeed(ctx, slug, q)
		if err != nil {
			return toolError(err, statusHint(err)), ExportOutput{}, nil
		}
		defer func() { _ = body.Close() }()

		var (
			rows      []string
			truncated bool
		)
		sc := bufio.NewScanner(body)
		// Export rows (especially JSON objects) can be long; give the scanner
		// room so a wide row is not a read error.
		sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for sc.Scan() {
			line := sc.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			if len(rows) >= sampleCap {
				truncated = true
				break
			}
			rows = append(rows, line)
		}
		// A scan error only matters when we intended to read to the end; if we
		// stopped early at the cap the unread remainder is expected.
		if err := sc.Err(); err != nil && !truncated {
			return toolError(fmt.Errorf("read export stream: %w", err), ""), ExportOutput{}, nil
		}

		return nil, ExportOutput{
			Feed:      feed.Name,
			Slug:      slug,
			Format:    format,
			Rows:      rows,
			RowCount:  len(rows),
			Truncated: truncated,
			Command:   exportCommand(slug, format, in),
		}, nil
	})
}

// exportCommand reconstructs the `spur export` invocation that streams the
// full result this tool call sampled: the resolved slug (which already carries
// any -ipv6 suffix, so no --ipv6 flag is needed) plus every filter that shaped
// the sample. The sampling limit is deliberately omitted — the command exists
// to fetch the unbounded stream. Values are shell-quoted so a model-supplied
// filter can never smuggle shell syntax into a command a user may paste.
func exportCommand(slug, format string, in ExportInput) string {
	args := []string{"spur", "export", shellQuote(slug)}
	if format != "" && format != "csv" {
		args = append(args, "--format", shellQuote(format))
	}
	if len(in.Fields) > 0 {
		args = append(args, "--fields", shellQuote(strings.Join(in.Fields, ",")))
	}
	if len(in.Tags) > 0 {
		args = append(args, "--tags", shellQuote(strings.Join(in.Tags, ",")))
	}
	if len(in.Ccs) > 0 {
		args = append(args, "--ccs", shellQuote(strings.Join(in.Ccs, ",")))
	}
	if len(in.Asns) > 0 {
		strs := make([]string, len(in.Asns))
		for i, a := range in.Asns {
			strs[i] = strconv.Itoa(a)
		}
		args = append(args, "--asns", strings.Join(strs, ","))
	}
	if len(in.CallbackProxies) > 0 {
		args = append(args, "--callback-proxies", shellQuote(strings.Join(in.CallbackProxies, ",")))
	}
	if in.Attributed != nil {
		args = append(args, "--attributed="+strconv.FormatBool(*in.Attributed))
	}
	if in.DiffDate != "" {
		args = append(args, "--diff-date", shellQuote(in.DiffDate))
	} else if in.Diff {
		args = append(args, "--diff")
	}
	if in.CSVHeader {
		args = append(args, "--csv-header")
	}
	return strings.Join(args, " ")
}

// shellSafe matches strings that need no quoting as a shell word.
var shellSafe = regexp.MustCompile(`^[A-Za-z0-9@%_+=:,./-]+$`)

// shellQuote returns s unchanged when it is safe as a bare shell word and
// single-quoted (with embedded single quotes escaped) otherwise.
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
