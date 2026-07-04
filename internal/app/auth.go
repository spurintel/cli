package app

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/auth"
)

// errAuthCancelled signals that the user declined to overwrite an existing
// token. It is a clean cancellation, not a failure, so RunE prints a note and
// returns nil rather than surfacing an error.
var errAuthCancelled = errors.New("auth cancelled")

// newAuthCmd builds `spur auth`: interactively prompt for an API token with
// hidden input and store it in the OS keychain, or with --logout remove the
// stored token. The keychain is the lowest-precedence source the token resolver
// consults, so a token stored here is used whenever SPUR_TOKEN and the config
// file supply none.
func newAuthCmd() *cobra.Command {
	var logout bool

	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Store your Spur API token securely in the OS keychain",
		Long: "Store your Spur API token in the operating system keychain so you " +
			"never have to keep it in your shell history or environment.\n\n" +
			"Run without flags to be prompted for the token with hidden input; if a " +
			"token is already stored you are asked before it is overwritten. Use " +
			"--logout to remove the stored token.\n\n" +
			"The keychain is the lowest-precedence token source: SPUR_TOKEN and the " +
			"config file still take precedence over a token stored here.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			kr := auth.DefaultKeyring()

			if logout {
				if err := auth.Logout(kr); err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), "Logged out: removed the stored token from the keychain.")
				return nil
			}

			token, err := promptForToken(kr)
			if errors.Is(err, errAuthCancelled) {
				fmt.Fprintln(cmd.OutOrStdout(), "Kept the existing token; nothing changed.")
				return nil
			}
			if err != nil {
				return err
			}
			if err := auth.Store(kr, token); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Token stored in the OS keychain.")
			return nil
		},
	}

	cmd.Flags().BoolVar(&logout, "logout", false, "remove the stored token from the keychain")
	return cmd
}

// promptForToken interactively reads a token with hidden input. When a token is
// already stored it first asks for confirmation before overwriting, returning
// errAuthCancelled if the user declines. The returned token is trimmed and
// guaranteed non-empty.
func promptForToken(kr auth.Keyring) (string, error) {
	if existing, err := kr.Get(); err == nil && existing != "" {
		overwrite := false
		if err := huh.NewConfirm().
			Title("A Spur token is already stored. Overwrite it?").
			Affirmative("Overwrite").
			Negative("Keep existing").
			Value(&overwrite).
			Run(); err != nil {
			return "", err
		}
		if !overwrite {
			return "", errAuthCancelled
		}
	}

	var token string
	if err := huh.NewInput().
		Title("Spur API token").
		Description("Paste your token from https://app.spur.us/settings/api (input hidden)").
		EchoMode(huh.EchoModePassword).
		Value(&token).
		Run(); err != nil {
		return "", err
	}

	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("no token entered")
	}
	return token, nil
}
