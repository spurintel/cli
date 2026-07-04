package app

import (
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/output"
	"github.com/spurintel/cli/internal/spur"
)

// newFeedCmd builds the `spur feed` command group for the Spur Data Feeds
// product (bulk intelligence delivered from feeds.spur.us, a distinct host from
// the Context API). It has no action of its own; `feed list` prints the catalog
// and `feed status <type>` fetches a feed's metadata document.
func newFeedCmd(of *outputFlags, cf *configFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feed",
		Short: "Inspect Spur data feeds (catalog and metadata)",
		Long: "Work with the Spur Data Feeds product, which delivers bulk " +
			"intelligence as newline-delimited JSON (gzip) from feeds.spur.us.\n\n" +
			"`spur feed list` prints the supported feed types; `spur feed status " +
			"<type>` reports the current feed's date, generation timestamps, and " +
			"size; `spur feed download <type>` streams a feed's data file.",
	}
	cmd.AddCommand(newFeedListCmd(of))
	cmd.AddCommand(newFeedStatusCmd(of, cf))
	cmd.AddCommand(newFeedDownloadCmd(of, cf))
	return cmd
}

// newFeedListCmd builds `spur feed list`: print the built-in feed catalog. It
// needs no token or network call — the catalog is static — but still routes
// through the renderer so it honors --format like every other command.
func newFeedListCmd(of *outputFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the supported Spur feed types",
		Long: "List the supported Spur data-feed types, including the IPv6 and " +
			"realtime variants. Each entry's name is what you pass to `spur feed " +
			"status <type>`.\n\n" +
			"Output is a styled report at a terminal and JSON when piped or " +
			"redirected; use --format to override.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return of.emit(cmd, spur.FeedTypes)
		},
	}
}

// newFeedStatusCmd builds `spur feed status <type>` (alias `metadata`): fetch
// the feed's `.../latest` metadata document and render its location, date,
// generation timestamps, and filesize. The result renders as a styled report at
// a terminal and as JSON when piped or redirected (override with --format).
func newFeedStatusCmd(of *outputFlags, cf *configFlags) *cobra.Command {
	return &cobra.Command{
		Use:     "status <type>",
		Aliases: []string{"metadata"},
		Short:   "Show a feed's current metadata (date, timestamps, filesize)",
		Long: "Fetch the metadata document for a Spur data feed and report the " +
			"current file's date, when it was generated and made available, its " +
			"relative location, and its size. Run `spur feed list` for the feed " +
			"types you can pass.\n\n" +
			"Output is a styled report at a terminal and JSON when piped or " +
			"redirected; use --format to override.\n\n" +
			"Authentication resolves a token from SPUR_TOKEN, then the config file, " +
			"then the OS keychain; run `spur auth` to store one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			feedType, ok := spur.LookupFeedType(args[0])
			if !ok {
				return fmt.Errorf("unknown feed type %q; run 'spur feed list' to see the supported feeds", args[0])
			}

			token, err := resolveToken(cf.resolved.Token)
			if err != nil {
				return err
			}
			client := cf.newClient(token)

			md, err := fetchFeedMetadata(cmd.Context(), client, feedType.Name)
			if err != nil {
				return err
			}
			return of.emit(cmd, md)
		},
	}
}

// newFeedDownloadCmd builds `spur feed download <type>`: stream a feed's gzip
// data file from feeds.spur.us. Without -o the body is decompressed to
// newline-delimited JSON on stdout (so it pipes straight into a JSON tool);
// with -o the raw gzip is written to the file (add --decompress to expand it).
// The download bypasses the renderer entirely — it is a byte stream, not a
// structured value — so --format and --no-color do not apply.
func newFeedDownloadCmd(of *outputFlags, cf *configFlags) *cobra.Command {
	var (
		date       string
		ipv6       bool
		realtime   bool
		asMMDB     bool
		asJSON     bool
		decompress bool
		silent     bool
	)
	cmd := &cobra.Command{
		Use:   "download <type>",
		Short: "Download a feed's data file (via the CDN)",
		Long: "Download a Spur data feed's file from feeds.spur.us. The request is " +
			"authenticated with your token and follows the redirect to the CDN " +
			"automatically.\n\n" +
			"Feeds are published as newline-delimited JSON (gzip); some are also " +
			"published as a MaxMind DB (.mmdb). The JSON gzip is the default, except " +
			"for `ipgeo`, which defaults to MMDB. Use --mmdb or --json to choose " +
			"explicitly; whether a feed offers MMDB is reported by `spur feed " +
			"status`.\n\n" +
			"For the JSON format the gzip body is decompressed to newline-delimited " +
			"JSON on stdout by default, so it pipes straight into a JSON tool. With " +
			"-o the raw `.json.gz` is written (fast, no decompression); pass " +
			"--decompress to expand it. MMDB is a binary file: it is written raw and " +
			"needs -o (it is never written to a terminal). Large feeds stream " +
			"without buffering the whole file in memory.\n\n" +
			"Use --date YYYYMMDD for a historical release, and --ipv6 or --realtime " +
			"to select a feed's IPv6 or realtime variant. Run `spur feed list` for " +
			"the feed types you can pass.\n\n" +
			"Download progress is shown on stderr at a terminal and suppressed with " +
			"--silent or when stderr is not a terminal, so stdout stays a clean data " +
			"channel.\n\n" +
			"Authentication resolves a token from SPUR_TOKEN, then the config file, " +
			"then the OS keychain; run `spur auth` to store one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, err := resolveDownloadSlug(args[0], ipv6, realtime)
			if err != nil {
				return err
			}
			if date != "" && strings.HasSuffix(slug, "/realtime") {
				return fmt.Errorf("--date is not supported for the realtime feed; realtime history is addressed by minute, which this command does not expose")
			}
			format, err := resolveDownloadFormat(slug, asMMDB, asJSON)
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

			toStdout := of.output == "" || of.output == "-"
			// MMDB is binary; dumping it to a terminal is never what the user
			// wants and would corrupt the display.
			if format == spur.FeedMMDB && toStdout && output.IsTerminal(dst) {
				return fmt.Errorf("refusing to write a binary MMDB file to the terminal; use -o <file>")
			}
			// stdout gets readable JSON by default; a file gets the raw gzip
			// unless the user asks to expand it. (Ignored for MMDB, which is
			// never gzip.)
			expand := toStdout || decompress

			// Progress is a diagnostic: stderr only, and only when a human is
			// watching. It never touches the stdout data channel.
			var progressOut io.Writer
			if !silent && output.IsTerminal(os.Stderr) {
				progressOut = cmd.ErrOrStderr()
			}

			return downloadFeed(cmd.Context(), client, slug, date, format, dst, expand, progressOut)
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "download a historical release for this date (YYYYMMDD) instead of the latest")
	cmd.Flags().BoolVar(&ipv6, "ipv6", false, "select the feed's IPv6 variant (e.g. anonymous -> anonymous-ipv6)")
	cmd.Flags().BoolVar(&realtime, "realtime", false, "select the feed's realtime variant (anonymous-residential only)")
	cmd.Flags().BoolVar(&asMMDB, "mmdb", false, "download the MaxMind DB (.mmdb) artifact where offered (default for ipgeo)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "download the newline-JSON gzip artifact (overrides the ipgeo MMDB default)")
	cmd.Flags().BoolVar(&decompress, "decompress", false, "decompress the gzip when writing JSON to a file (-o); stdout JSON is always decompressed")
	cmd.Flags().BoolVar(&silent, "silent", false, "suppress the download progress indicator")
	return cmd
}

// resolveDownloadFormat picks the artifact format from the explicit flags,
// falling back to a per-feed default: ipgeo is a geolocation database whose
// primary form is MMDB, so it defaults to MMDB; every other feed defaults to the
// newline-JSON gzip. --mmdb and --json are mutually exclusive. Whether the feed
// actually offers the chosen format is validated later against its metadata.
func resolveDownloadFormat(slug string, mmdb, jsonFmt bool) (spur.FeedFormat, error) {
	switch {
	case mmdb && jsonFmt:
		return 0, fmt.Errorf("--mmdb and --json are mutually exclusive")
	case mmdb:
		return spur.FeedMMDB, nil
	case jsonFmt:
		return spur.FeedJSONGzip, nil
	case slug == "ipgeo":
		return spur.FeedMMDB, nil
	default:
		return spur.FeedJSONGzip, nil
	}
}

// resolveDownloadSlug turns the user's feed argument plus the --ipv6/--realtime
// modifiers into a catalog feed slug, then validates it. The modifiers are
// convenience sugar: `anonymous --ipv6` resolves to anonymous-ipv6 and
// `anonymous-residential --realtime` to anonymous-residential/realtime, but the
// full slug may also be passed directly. An unrecognized result (including an
// impossible modifier combination) is rejected before any network call.
func resolveDownloadSlug(arg string, ipv6, realtime bool) (string, error) {
	slug := strings.Trim(strings.TrimSpace(arg), "/")
	if ipv6 && !strings.HasSuffix(slug, "-ipv6") {
		slug += "-ipv6"
	}
	if realtime && !strings.HasSuffix(slug, "/realtime") {
		slug += "/realtime"
	}
	ft, ok := spur.LookupFeedType(slug)
	if !ok {
		return "", fmt.Errorf("unknown feed type %q; run 'spur feed list' to see the supported feeds", slug)
	}
	return ft.Name, nil
}

// downloadFeed streams a feed file from the client to dst, optionally expanding
// the gzip and reporting progress. Like fetchFeedMetadata it is split from the
// cobra wiring so it can be exercised end-to-end against an httptest server, and
// it collapses a forbidden status to the same feed-specific, actionable message.
//
// For MMDB it first consults the feed's metadata: whether a feed publishes an
// MMDB artifact is authoritative there (the MMDB container), so a feed that has
// no MMDB gets a clear up-front error instead of a bare 404 mid-download.
//
// progressOut, when non-nil, receives a self-overwriting progress line; the
// counter wraps the transfer stream so it tracks bytes downloaded against the
// CDN's Content-Length.
func downloadFeed(ctx context.Context, client *spur.Client, feedType, date string, format spur.FeedFormat, dst io.Writer, expand bool, progressOut io.Writer) error {
	if format == spur.FeedMMDB {
		md, err := fetchFeedMetadata(ctx, client, feedType)
		if err != nil {
			return err
		}
		if md.MMDB == nil {
			return fmt.Errorf("the %q feed is not offered in MMDB format; download it as JSON instead (omit --mmdb)", feedType)
		}
	}

	body, size, err := client.DownloadFeed(ctx, feedType, date, format)
	if err != nil {
		var apiErr *spur.APIError
		if errors.As(err, &apiErr) {
			if errors.Is(apiErr, spur.ErrForbidden) {
				return fmt.Errorf("access to the %q feed is denied: your token's subscription does not include it", feedType)
			}
			return errors.New(apiErr.Message())
		}
		return err
	}
	defer func() { _ = body.Close() }()

	var src io.Reader = body
	var prog *progressReader
	if progressOut != nil {
		prog = newProgressReader(src, progressOut, "Downloading "+feedType, size)
		src = prog
	}

	// Only the JSON artifact is gzip; MMDB is written through untouched.
	if format == spur.FeedJSONGzip && expand {
		gz, err := gzip.NewReader(src)
		if err != nil {
			return fmt.Errorf("decompress feed: %w", err)
		}
		defer func() { _ = gz.Close() }()
		src = gz
	}

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("stream feed: %w", err)
	}
	if prog != nil {
		prog.Finish()
	}
	return nil
}

// fetchFeedMetadata calls the client and collapses an API failure to a clear
// cause. A forbidden status becomes a feed-specific, actionable message (the
// token's subscription doesn't include this feed); the other classes surface
// the client's stable, sanitized message. It is split from the cobra wiring and
// from rendering so it can be exercised end-to-end against an httptest server.
func fetchFeedMetadata(ctx context.Context, client *spur.Client, feedType string) (spur.FeedMetadata, error) {
	md, err := client.GetFeedMetadata(ctx, feedType)
	if err != nil {
		var apiErr *spur.APIError
		if errors.As(err, &apiErr) {
			if errors.Is(apiErr, spur.ErrForbidden) {
				return spur.FeedMetadata{}, fmt.Errorf("access to the %q feed is denied: your token's subscription does not include it", feedType)
			}
			return spur.FeedMetadata{}, errors.New(apiErr.Message())
		}
		return spur.FeedMetadata{}, err
	}
	return md, nil
}
