// Package app wires config, logging, and the CLI root command.
package app

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

// Run builds the spur CLI and executes it against ctx. cmd/spur/main.go calls
// this. The context carries OS-signal cancellation down into the commands so a
// long-running request is interrupted on Ctrl-C.
func Run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	// Diagnostics go to stderr so stdout stays a clean data channel (the
	// context command prints JSON there).
	slog.SetDefault(newLogger(os.Stderr))
	_ = cfg // wire into subcommands as needed

	return newRootCmd().ExecuteContext(ctx)
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spur",
		Short: "Spur command-line client",
		Long:  "spur is a command-line client for the Spur Context API.",
		// A usage dump on a runtime error (e.g. a bad token) is noise; the
		// error message alone is what the operator needs. SilenceErrors lets
		// cmd/spur/main.go be the single place that prints the failure, so it
		// is not reported twice.
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	of := &outputFlags{}
	of.register(cmd)
	cf := &configFlags{}
	cf.register(cmd)
	// Validate output flags and resolve config once, before any command does
	// network work, so a bad --format or unknown --profile never spends a
	// metered API query.
	cmd.PersistentPreRunE = func(*cobra.Command, []string) error {
		if err := of.validate(); err != nil {
			return err
		}
		return cf.resolve()
	}
	cmd.AddCommand(newContextCmd(of, cf))
	cmd.AddCommand(newStatusCmd(of, cf))
	cmd.AddCommand(newMCPCmd(cf))
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newCompletionCmd())
	return cmd
}
