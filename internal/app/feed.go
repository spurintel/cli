package app

import (
	"compress/gzip"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"hash"
	"hash/crc32"
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
		Short:   "Show a feed's current metadata (date, timestamps, filesize, checksums)",
		Long: "Fetch the metadata document for a Spur data feed and report the " +
			"current file's date, when it was generated and made available, its " +
			"relative location, its size, and its checksums (CRC32C/MD5) as " +
			"reported by the CDN. Run `spur feed list` for the feed types you can " +
			"pass.\n\n" +
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
			attachFeedDigests(cmd.Context(), client, feedType.Name, &md)
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
		verify     bool
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
			"After the download, a size and checksum (CRC32C/MD5) summary as reported " +
			"by the CDN is printed to stderr, unless --silent. Pass --verify to hash " +
			"the downloaded bytes as they arrive and compare them to that reported " +
			"checksum, failing the command on a mismatch.\n\n" +
			"Authentication resolves a token from SPUR_TOKEN, then the config file, " +
			"then the OS keychain; run `spur auth` to store one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug, err := resolveDownloadSlug(args[0], ipv6, realtime)
			if err != nil {
				return err
			}
			if _, err := spur.ValidateHistoricalDate(date); err != nil {
				return fmt.Errorf("invalid --date %q; want YYYYMMDD", date)
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

			// MMDB is binary; dumping it to a terminal is never what the user
			// wants and would corrupt the display.
			if format == spur.FeedMMDB && of.toStdout() && output.IsTerminal(cmd.OutOrStdout()) {
				return fmt.Errorf("refusing to write a binary MMDB file to the terminal; use -o <file>")
			}
			// stdout gets readable JSON by default; a file gets the raw gzip
			// unless the user asks to expand it. (Ignored for MMDB, which is
			// never gzip.)
			expand := of.toStdout() || decompress

			// Progress is a diagnostic: stderr only, and only when a human is
			// watching. It never touches the stdout data channel.
			var progress *progressLine
			if !silent && output.IsTerminal(os.Stderr) {
				w := cmd.ErrOrStderr()
				progress = newProgressLine(w, of.colorEnabled(w), "Downloading "+slug)
			}

			// The integrity summary is a diagnostic like progress, but unlike
			// progress it is shown whenever a human isn't opting out with
			// --silent, even when stderr is piped (CI logs still want it).
			var summary io.Writer
			if !silent {
				summary = cmd.ErrOrStderr()
			}

			return downloadFeed(cmd.Context(), client, slug, date, format, of.sink(cmd), expand, progress, verify, summary)
		},
	}
	cmd.Flags().StringVar(&date, "date", "", "download a historical release for this date (YYYYMMDD) instead of the latest")
	cmd.Flags().BoolVar(&ipv6, "ipv6", false, "select the feed's IPv6 variant (e.g. anonymous -> anonymous-ipv6)")
	cmd.Flags().BoolVar(&realtime, "realtime", false, "select the feed's realtime variant (anonymous-residential only)")
	cmd.Flags().BoolVar(&asMMDB, "mmdb", false, "download the MaxMind DB (.mmdb) artifact where offered (default for ipgeo)")
	cmd.Flags().BoolVar(&asJSON, "json", false, "download the newline-JSON gzip artifact (overrides the ipgeo MMDB default)")
	cmd.Flags().BoolVar(&decompress, "decompress", false, "decompress the gzip when writing JSON to a file (-o); stdout JSON is always decompressed")
	cmd.Flags().BoolVar(&silent, "silent", false, "suppress the download progress indicator")
	cmd.Flags().BoolVar(&verify, "verify", false, "hash the downloaded bytes and verify them against the CDN-reported checksum (crc32c/md5); a mismatch fails the command but a partially-written -o file is left in place")
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

// downloadFeed streams a feed file from the client to the destination opened
// by openSink, optionally expanding the gzip and reporting progress. Like
// fetchFeedMetadata it is split from the cobra wiring so it can be exercised
// end-to-end against an httptest server, and it collapses a forbidden status
// to the same feed-specific, actionable message.
//
// The sink is opened only after the download request has succeeded, so a
// failed request (bad token, missing date, rate limit) never creates or
// truncates the -o file; a close failure surfaces as the command error since
// it means the file is incomplete.
//
// For MMDB it first consults the feed's metadata: whether a feed publishes an
// MMDB artifact is authoritative there (the MMDB container), so a feed that has
// no MMDB gets a clear up-front error instead of a bare 404 mid-download.
//
// progress, when non-nil, receives a self-overwriting progress bar; the
// counter wraps the transfer stream so it tracks bytes downloaded against the
// CDN's Content-Length.
//
// When verify is true the RAW bytes as received from the CDN (before any
// progress wrapping or gzip decompression) are hashed with CRC32C and MD5 and
// compared against the CDN-reported digests (x-goog-hash, surfaced as
// integ.CRC32C/integ.MD5) once the transfer completes; a mismatch, or a
// request to verify against a response that reported no digest at all, fails
// the command. summary, when non-nil, receives a one-line size/checksum
// report (as reported by the CDN, regardless of verify) plus a second line
// noting which digests were verified when verify is true.
func downloadFeed(
	ctx context.Context, client *spur.Client, feedType, date string, format spur.FeedFormat,
	openSink func() (io.Writer, func() error, error), expand bool, progress *progressLine,
	verify bool, summary io.Writer,
) error {
	if format == spur.FeedMMDB {
		md, err := fetchFeedMetadata(ctx, client, feedType)
		if err != nil {
			return err
		}
		if md.MMDB == nil {
			return fmt.Errorf("the %q feed is not offered in MMDB format; download it as JSON instead (omit --mmdb)", feedType)
		}
	}

	body, integ, err := client.DownloadFeed(ctx, feedType, date, format)
	if err != nil {
		return collapseAPIError(err, map[error]string{
			spur.ErrForbidden: fmt.Sprintf("access to the %q feed is denied: your token's subscription does not include it", feedType),
		})
	}
	defer func() { _ = body.Close() }()

	var src io.Reader = body

	// Hash the RAW bytes as received from the CDN: x-goog-hash is computed
	// over the stored object (.json.gz/.mmdb), so this tee must sit before
	// both the progress wrapper and any gzip decompression, or it would hash
	// the wrong bytes.
	var crc32cHash hash.Hash32
	var md5Hash hash.Hash
	if verify {
		crc32cHash = crc32.New(crc32.MakeTable(crc32.Castagnoli))
		md5Hash = md5.New()
		src = io.TeeReader(src, io.MultiWriter(crc32cHash, md5Hash))
	}

	var prog *progressReader
	if progress != nil {
		prog = newProgressReader(src, progress, integ.Size)
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

	dst, closeOut, err := openSink()
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = closeOut()
		return fmt.Errorf("stream feed: %w", err)
	}
	if err := closeOut(); err != nil {
		return fmt.Errorf("close output: %w", err)
	}
	if prog != nil {
		prog.Finish()
	}

	if verify {
		if err := verifyFeedIntegrity(integ, crc32cHash, md5Hash); err != nil {
			return err
		}
	}

	if summary != nil {
		writeFeedIntegritySummary(summary, integ, verify)
	}
	return nil
}

// verifyFeedIntegrity compares the locally-computed CRC32C/MD5 digests
// against the CDN-reported ones in integ, checking only the digests the CDN
// actually reported. A caller that asked to verify against a response
// reporting neither digest gets a clear error rather than a silent no-op.
func verifyFeedIntegrity(integ spur.FeedIntegrity, crc32cHash hash.Hash32, md5Hash hash.Hash) error {
	if integ.CRC32C == "" && integ.MD5 == "" {
		return fmt.Errorf("--verify requested but the CDN did not report a checksum to verify against")
	}
	if integ.CRC32C != "" {
		var sum [4]byte
		binary.BigEndian.PutUint32(sum[:], crc32cHash.Sum32())
		if got := base64.StdEncoding.EncodeToString(sum[:]); got != integ.CRC32C {
			return fmt.Errorf("feed integrity check failed: crc32c mismatch (expected %s, got %s)", integ.CRC32C, got)
		}
	}
	if integ.MD5 != "" {
		if got := base64.StdEncoding.EncodeToString(md5Hash.Sum(nil)); got != integ.MD5 {
			return fmt.Errorf("feed integrity check failed: md5 mismatch (expected %s, got %s)", integ.MD5, got)
		}
	}
	return nil
}

// writeFeedIntegritySummary prints a one-line size/checksum report as
// reported by the CDN (regardless of whether verify ran), and, when verified
// is true, a second line naming which digests were checked.
func writeFeedIntegritySummary(w io.Writer, integ spur.FeedIntegrity, verified bool) {
	parts := []string{sizeSummary(integ.Size)}
	var checked []string
	if integ.CRC32C != "" {
		parts = append(parts, "crc32c="+integ.CRC32C)
		checked = append(checked, "crc32c")
	}
	if integ.MD5 != "" {
		parts = append(parts, "md5="+integ.MD5)
		checked = append(checked, "md5")
	}
	fmt.Fprintln(w, strings.Join(parts, "  "))
	if verified {
		fmt.Fprintf(w, "integrity verified (%s)\n", strings.Join(checked, ", "))
	}
}

// sizeSummary renders integ.Size for the summary line: "size=unknown" when
// the CDN did not report a size (-1), else the byte count.
func sizeSummary(size int64) string {
	if size < 0 {
		return "size=unknown"
	}
	return fmt.Sprintf("size=%d bytes", size)
}

// attachFeedDigests best-effort populates the CRC32C/MD5 of each artifact in
// md by probing the CDN with ProbeFeedIntegrity. The metadata document itself
// never carries these digests; this is a client-side probe of the artifact
// (accepted tradeoff: one extra request per artifact). A probe failure leaves
// that artifact's digests empty and is otherwise ignored — it never fails the
// status command.
func attachFeedDigests(ctx context.Context, client *spur.Client, feedType string, md *spur.FeedMetadata) {
	if integ, err := client.ProbeFeedIntegrity(ctx, feedType, "", spur.FeedJSONGzip); err == nil {
		md.JSON.CRC32C, md.JSON.MD5 = integ.CRC32C, integ.MD5
	}
	if md.MMDB != nil {
		if integ, err := client.ProbeFeedIntegrity(ctx, feedType, "", spur.FeedMMDB); err == nil {
			md.MMDB.CRC32C, md.MMDB.MD5 = integ.CRC32C, integ.MD5
		}
	}
}

// fetchFeedMetadata calls the client and collapses an API failure to a clear
// cause. A forbidden status becomes a feed-specific, actionable message (the
// token's subscription doesn't include this feed); the other classes surface
// the client's stable, sanitized message. It is split from the cobra wiring and
// from rendering so it can be exercised end-to-end against an httptest server.
func fetchFeedMetadata(ctx context.Context, client *spur.Client, feedType string) (spur.FeedMetadata, error) {
	md, err := client.GetFeedMetadata(ctx, feedType)
	if err != nil {
		return spur.FeedMetadata{}, collapseAPIError(err, map[error]string{
			spur.ErrForbidden: fmt.Sprintf("access to the %q feed is denied: your token's subscription does not include it", feedType),
		})
	}
	return md, nil
}
