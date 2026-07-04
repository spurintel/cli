package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

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
			"size.",
	}
	cmd.AddCommand(newFeedListCmd(of))
	cmd.AddCommand(newFeedStatusCmd(of, cf))
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
