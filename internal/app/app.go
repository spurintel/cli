// Package app wires config, logging, and the CLI root command.
package app

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/output"
)

// Run builds the spur CLI and executes it against ctx. cmd/spur/main.go calls
// this. The context carries OS-signal cancellation down into the commands so a
// long-running request is interrupted on Ctrl-C.
func Run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	// Diagnostics go to stderr so stdout contains only the requested command
	// output, including machine-readable data when piped.
	slog.SetDefault(newLogger(os.Stderr, parseLogLevel(cfg.LogLevel)))

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
	cmd.PersistentPreRunE = func(c *cobra.Command, _ []string) error {
		if err := of.validate(); err != nil {
			return err
		}
		// Commands that never consume the config must not be blocked by a
		// malformed config file: completion runs from shell init (a TOML typo
		// would break every new shell), and `spur auth --logout` is a
		// recovery path.
		if !needsConfig(c) {
			return nil
		}
		if err := cf.resolve(); err != nil {
			return err
		}
		if cf.warning != "" {
			w := c.ErrOrStderr()
			output.Warnf(w, of.colorEnabled(w), "%s", cf.warning)
		}
		return nil
	}
	cmd.AddCommand(newContextCmd(of, cf))
	cmd.AddCommand(newStatusCmd(of, cf))
	cmd.AddCommand(newTagCmd(of, cf))
	cmd.AddCommand(newFeedCmd(of, cf))
	cmd.AddCommand(newExportCmd(of, cf))
	cmd.AddCommand(newAuthCmd(of))
	cmd.AddCommand(newMCPCmd(cf))
	cmd.AddCommand(newVersionCmd())
	cmd.AddCommand(newCompletionCmd())
	return cmd
}

// needsConfig reports whether the invoked command (or any of its ancestors)
// consumes the resolved config. auth, version, completion, help, and cobra's
// hidden shell-completion machinery are config-free by design.
func needsConfig(c *cobra.Command) bool {
	for cur := c; cur != nil; cur = cur.Parent() {
		switch cur.Name() {
		case "auth", "version", "completion", "help",
			cobra.ShellCompRequestCmd, cobra.ShellCompNoDescRequestCmd:
			return false
		}
	}
	return true
}
