package app

import (
	"context"
	"errors"
	"os"

	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/spur"
)

// newContextCmd builds `spur context <ip>`: look up an IP in the Spur Context
// API and render the result through the shared output renderer (styled text at
// a terminal, JSON when piped; overridable with --format).
func newContextCmd(of *outputFlags, cf *configFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "context <ip>",
		Short: "Look up the Spur context for an IP address",
		Long: "Look up an IPv4 or IPv6 address in the Spur Context API.\n\n" +
			"Output is a styled report at a terminal and newline-delimited JSON when " +
			"piped or redirected; use --format to override.\n\n" +
			"Authentication uses the SPUR_TOKEN environment variable.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := resolveToken()
			if err != nil {
				return err
			}
			client := cf.newClient(token)
			result, err := fetchContext(cmd.Context(), client, args[0])
			if err != nil {
				return err
			}
			return of.emit(cmd, result)
		},
	}
}

// resolveToken returns the Spur API token. For this cut it comes solely from
// the SPUR_TOKEN environment variable; config-file and keychain sources are
// deferred to a later ticket.
func resolveToken() (string, error) {
	if t := os.Getenv("SPUR_TOKEN"); t != "" {
		return t, nil
	}
	return "", errors.New("SPUR_TOKEN is not set; get a token from https://app.spur.us/settings/api")
}

// fetchContext validates the IP and calls the client, returning the context.
// It is split from the cobra wiring and from rendering so it can be exercised
// end-to-end against an httptest server.
//
// API failures are collapsed to the client's stable, sanitized message
// (unauthorized / forbidden / throttled / bad request) so the operator sees a
// clear cause instead of a raw status line or a stack trace.
func fetchContext(ctx context.Context, client *spur.Client, ip string) (spur.IPContext, error) {
	ip, err := spur.ValidateIP(ip)
	if err != nil {
		return spur.IPContext{}, err
	}

	result, _, err := client.GetIPContext(ctx, ip, spur.IPContextOptions{})
	if err != nil {
		var apiErr *spur.APIError
		if errors.As(err, &apiErr) {
			return spur.IPContext{}, errors.New(apiErr.Message())
		}
		return spur.IPContext{}, err
	}
	return result, nil
}
