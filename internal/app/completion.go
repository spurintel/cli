package app

import "github.com/spf13/cobra"

// newCompletionCmd builds `spur completion [bash|zsh|fish]`, writing a shell
// completion script to stdout.
//
// This lightly customizes Cobra's built-in completion machinery rather than
// re-implementing it: the hidden `__complete` command Cobra registers at
// execution time powers the actual runtime tab-completion, and the per-shell
// scripts emitted here are the bootstrap the user sources to wire that hook
// into their shell. Only bash, zsh, and fish are offered (Cobra's default
// command also emits PowerShell) to match the shells Spur supports and the
// acceptance criteria for this command.
func newCompletionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "completion [bash|zsh|fish]",
		Short: "Generate a shell completion script",
		Long: "Generate a shell completion script for spur.\n\n" +
			"Load completions into the current shell session:\n\n" +
			"  Bash:\n" +
			"    source <(spur completion bash)\n\n" +
			"  Zsh:\n" +
			"    source <(spur completion zsh)\n\n" +
			"  Fish:\n" +
			"    spur completion fish | source\n\n" +
			"To load completions on every new session, write the script to your " +
			"shell's completion directory; see your shell's documentation for the " +
			"path. The Homebrew install bundles these automatically.",
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		ValidArgs:             []string{"bash", "zsh", "fish"},
		DisableFlagsInUseLine: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Completions describe the whole command tree, so generate against
			// the root regardless of how this subcommand was reached.
			root := cmd.Root()
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return root.GenBashCompletionV2(out, true)
			case "zsh":
				return root.GenZshCompletion(out)
			case "fish":
				return root.GenFishCompletion(out, true)
			}
			return nil
		},
	}
}
