package app

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/spur"
)

// newTagCmd builds `spur tag <TAG>`: look up a Spur service tag (a proxy/VPN
// brand such as OXYLABS_PROXY) and render its metadata — categories, anonymity,
// supported platforms/protocols, and aggregate metrics. The result renders as a
// styled report at a terminal and as JSON when piped or redirected (override
// with --format), the same as every other data-producing command.
func newTagCmd(of *outputFlags, cf *configFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "tag <TAG>",
		Short: "Look up metadata for a Spur service tag (proxy/VPN brand)",
		Long: "Look up a Spur service tag such as OXYLABS_PROXY, PROTON_VPN, or " +
			"TOR_PROXY and report what Spur knows about the provider: its " +
			"categories, whether it anonymizes traffic, the platforms and protocols " +
			"it supports, and aggregate metrics across observed endpoints.\n\n" +
			"Output is a styled report at a terminal and JSON when piped or " +
			"redirected; use --format to override.\n\n" +
			"Authentication resolves a token from SPUR_TOKEN, then the config file, " +
			"then the OS keychain; run `spur auth` to store one.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := resolveToken(cf.resolved.Token)
			if err != nil {
				return err
			}
			client := cf.newClient(token)

			tag, err := fetchTagMetadata(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}
			return of.emit(cmd, tag)
		},
	}
}

// fetchTagMetadata calls the client and collapses an API failure to a clear
// cause. A not-found status becomes a tag-specific, actionable message (the
// generic "not found" sentinel doesn't say what wasn't found); the other
// classes surface the client's stable, sanitized message (unauthorized /
// forbidden / throttled). It is split from the cobra wiring and from rendering
// so it can be exercised end-to-end against an httptest server.
func fetchTagMetadata(ctx context.Context, client *spur.Client, tag string) (spur.TagMetadata, error) {
	result, err := client.GetTagMetadata(ctx, tag)
	if err != nil {
		return spur.TagMetadata{}, collapseAPIError(err, map[error]string{
			spur.ErrNotFound: fmt.Sprintf("unknown tag %q: no metadata found (tags are case-sensitive, e.g. OXYLABS_PROXY)", tag),
		})
	}
	return result, nil
}
