package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/output"
	"github.com/spurintel/cli/internal/spur"
)

// exportFlags carries the command-local flags for `spur export`. They map onto
// the Exports API query parameters via spur.BuildExportQuery; only the filters a
// given feed accepts are honored (the rest are rejected up front).
type exportFlags struct {
	format          string
	fields          []string
	tags            []string
	ccs             []string
	asns            []int
	callbackProxies []string
	attributed      bool
	diff            bool
	diffDate        string
	limit           int64
	csvHeader       bool
	ipv6            bool
}

// newExportCmd builds `spur export <feed>`: call the Exports API
// (exports.spur.us) for a dynamically filtered, reformatted subset of a feed and
// write the raw result to stdout or -o. The feeds are anonymous,
// anonymous-residential, ztna, dch, and ipgeo; --ipv6 selects the IPv6 variant
// of the families that have one.
//
// The response is a byte stream (CSV, newline-JSON, or a MaxMind DB), so it
// bypasses the renderer entirely — --format here selects the *server* output
// format (csv|json|mmdb), not the local text/json/csv renderer, and --no-color
// does not apply. Destination and the -o flag are the shared persistent ones.
func newExportCmd(of *outputFlags, cf *configFlags) *cobra.Command {
	var f exportFlags

	cmd := &cobra.Command{
		Use:   "export <feed>",
		Short: "Export a filtered subset of a Spur feed (via the Exports API)",
		Long: "Export a dynamically filtered and reformatted subset of a Spur feed " +
			"from the Exports API (exports.spur.us). The query is built from the " +
			"flags below and the result is written to stdout or -o.\n\n" +
			"Feeds: anonymous, anonymous-residential, ztna, dch, ipgeo. Add --ipv6 " +
			"for the IPv6 variant of anonymous, anonymous-residential, or ztna.\n\n" +
			"Not every feed accepts every filter: --callback-proxies is " +
			"residential-only, --diff/--diff-date apply to anonymous and ztna, and " +
			"dch/ipgeo take neither --tags, --limit, nor (dch) --ccs. A filter a " +
			"feed does not accept is rejected before any request is made.\n\n" +
			"--format selects the server's output format (csv, json, or mmdb), not " +
			"the local renderer; mmdb is binary and must go to a file via -o. Large " +
			"exports stream without buffering the whole result in memory.\n\n" +
			"The Exports API is rate-limited per token; a 429 is reported with a " +
			"clear message so you can back off and retry.\n\n" +
			"Authentication resolves a token from SPUR_TOKEN, then the config file, " +
			"then the OS keychain; run `spur auth` to store one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			feed, slug, err := spur.ResolveExportFeed(args[0], f.ipv6)
			if err != nil {
				return err
			}

			opts := spur.ExportOptions{
				Format:          f.format,
				Fields:          f.fields,
				Tags:            f.tags,
				Ccs:             f.ccs,
				Asns:            f.asns,
				CallbackProxies: f.callbackProxies,
				Diff:            f.diff,
				DiffDate:        f.diffDate,
				Limit:           f.limit,
				CSVHeader:       f.csvHeader,
			}
			// --attributed defaults to true; only send it when the user set it
			// explicitly, so the server default governs otherwise.
			if cmd.Flags().Changed("attributed") {
				opts.Attributed = &f.attributed
			}

			q, err := spur.BuildExportQuery(feed, opts)
			if err != nil {
				return err
			}

			token, err := resolveToken(cf.resolved.Token)
			if err != nil {
				return err
			}
			client := cf.newClient(token)

			dst, closeOut, err := of.writer(cmd)
			if err != nil {
				return err
			}
			defer func() { _ = closeOut() }()

			// MMDB is binary; writing it to a terminal is never intended and
			// would corrupt the display.
			toStdout := of.output == "" || of.output == "-"
			if f.format == "mmdb" && toStdout && output.IsTerminal(dst) {
				return fmt.Errorf("refusing to write a binary MMDB export to the terminal; use -o <file>")
			}

			return streamExport(cmd.Context(), client, feed.Name, slug, q, dst)
		},
	}

	cmd.Flags().StringVar(&f.format, "format", "csv",
		"server output format: csv, json, or mmdb")
	cmd.Flags().StringSliceVar(&f.fields, "fields", nil,
		"comma-separated fields to include (feed-specific; default is the feed's full set)")
	cmd.Flags().StringSliceVar(&f.tags, "tags", nil,
		"filter to IPs carrying any of these service tags (comma-separated)")
	cmd.Flags().StringSliceVar(&f.ccs, "ccs", nil,
		"filter to these ISO country codes (comma-separated)")
	cmd.Flags().IntSliceVar(&f.asns, "asns", nil,
		"filter to these AS numbers (comma-separated)")
	cmd.Flags().StringSliceVar(&f.callbackProxies, "callback-proxies", nil,
		"filter to these callback-proxy providers (anonymous-residential only)")
	cmd.Flags().BoolVar(&f.attributed, "attributed", true,
		"include only IPs attributed to a service; --attributed=false includes unattributed")
	cmd.Flags().BoolVar(&f.diff, "diff", false,
		"return only changes versus the previous day (anonymous, ztna)")
	cmd.Flags().StringVar(&f.diffDate, "diff-date", "",
		"return the diff against this date (YYYY-MM-DD; implies --diff)")
	cmd.Flags().Int64Var(&f.limit, "limit", 0,
		"cap the number of rows returned")
	cmd.Flags().BoolVar(&f.csvHeader, "csv-header", false,
		"prepend a header row to CSV output")
	cmd.Flags().BoolVar(&f.ipv6, "ipv6", false,
		"select the feed's IPv6 variant (anonymous, anonymous-residential, ztna)")

	return cmd
}

// streamExport calls the Exports client and copies the raw body to dst. It is
// split from the cobra wiring so it can be exercised end-to-end against an
// httptest server. API failures are collapsed to clear, actionable causes: a
// rate limit (429) and a forbidden (403) each get a feed-aware message, and the
// rest surface the client's stable, sanitized message — never a leaked upstream
// body.
func streamExport(ctx context.Context, client *spur.Client, feedName, slug string, q url.Values, dst io.Writer) error {
	body, _, err := client.ExportFeed(ctx, slug, q)
	if err != nil {
		var apiErr *spur.APIError
		if errors.As(err, &apiErr) {
			switch {
			case errors.Is(apiErr, spur.ErrThrottled):
				return fmt.Errorf("the Exports API rate limit was exceeded; wait a moment and retry (exports are rate-limited per token)")
			case errors.Is(apiErr, spur.ErrForbidden):
				return fmt.Errorf("access to the %q export is denied: your token's subscription does not include it", feedName)
			}
			return errors.New(apiErr.Message())
		}
		return err
	}
	defer func() { _ = body.Close() }()

	if _, err := io.Copy(dst, body); err != nil {
		return fmt.Errorf("stream export: %w", err)
	}
	return nil
}
