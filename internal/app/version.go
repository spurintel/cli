package app

import (
	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/buildinfo"
)

// newVersionCmd prints the ldflags-injected build string.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the spur version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println(buildinfo.String())
			return nil
		},
	}
}
