package app

import (
	"context"
	"errors"

	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/spur"
)

// newStatusCmd builds `spur status`: report whether the configured token is
// active, how many queries remain in the current billing cycle, and the service
// tier. The result renders as a styled report at a terminal and as JSON when
// piped or redirected (override with --format), the same as every other
// data-producing command.
func newStatusCmd(of *outputFlags, cf *configFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Report token status: active flag, queries remaining, service tier",
		Long: "Report the status of the configured Spur API token: whether it is " +
			"active, how many queries remain in the current billing cycle, and the " +
			"service tier.\n\n" +
			"Output is a styled report at a terminal and JSON when piped or " +
			"redirected; use --format to override.\n\n" +
			"Authentication uses the SPUR_TOKEN environment variable.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			token, err := resolveToken()
			if err != nil {
				return err
			}
			client := cf.newClient(token)

			status, err := fetchStatus(cmd.Context(), client)
			if err != nil {
				return err
			}
			return of.emit(cmd, status)
		},
	}
}

// fetchStatus calls the client and collapses an API failure to the client's
// stable, sanitized message (unauthorized / forbidden / throttled) so an
// invalid token yields a clear cause instead of a raw status line. It is split
// from the cobra wiring and from rendering so it can be exercised end-to-end
// against an httptest server.
func fetchStatus(ctx context.Context, client *spur.Client) (spur.Status, error) {
	status, err := client.GetStatus(ctx)
	if err != nil {
		var apiErr *spur.APIError
		if errors.As(err, &apiErr) {
			return spur.Status{}, errors.New(apiErr.Message())
		}
		return spur.Status{}, err
	}
	return status, nil
}
