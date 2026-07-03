package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/spur"
)

// newContextCmd builds `spur context <ip>`: look up an IP in the Spur Context
// API and print the result as JSON.
func newContextCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "context <ip>",
		Short: "Look up the Spur context for an IP address",
		Long: "Look up an IPv4 or IPv6 address in the Spur Context API and print " +
			"the result as JSON.\n\nAuthentication uses the SPUR_TOKEN environment variable.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := resolveToken()
			if err != nil {
				return err
			}
			client := spur.NewClient(token)
			return runContext(cmd.Context(), client, cmd.OutOrStdout(), args[0])
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

// runContext validates the IP, calls the client, and writes indented JSON to w.
// It is split out from the cobra wiring so it can be exercised end-to-end in a
// test against an httptest server.
//
// API failures are collapsed to the client's stable, sanitized message
// (unauthorized / forbidden / throttled / bad request) so the operator sees a
// clear cause instead of a raw status line or a stack trace.
func runContext(ctx context.Context, client *spur.Client, w io.Writer, ip string) error {
	ip, err := spur.ValidateIP(ip)
	if err != nil {
		return err
	}

	result, _, err := client.GetIPContext(ctx, ip, spur.IPContextOptions{})
	if err != nil {
		var apiErr *spur.APIError
		if errors.As(err, &apiErr) {
			return errors.New(apiErr.Message())
		}
		return err
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	return nil
}
