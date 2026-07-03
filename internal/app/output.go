package app

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/spurintel/cli/internal/output"
)

// outputFlags backs the root command's persistent output flags. Every command
// that produces data routes through emit so format, destination, and color are
// resolved the same way everywhere.
type outputFlags struct {
	format  string // --format; empty means auto-detect from the destination
	output  string // --output/-o; "-" or empty means stdout
	noColor bool   // --no-color
}

// register binds the flags onto cmd's persistent flag set.
func (f *outputFlags) register(cmd *cobra.Command) {
	pf := cmd.PersistentFlags()
	pf.StringVar(&f.format, "format", "",
		fmt.Sprintf("output format [%s] (default: text at a terminal, json when piped)", output.FormatList()))
	pf.StringVarP(&f.output, "output", "o", "-",
		"write output to a file instead of stdout ('-' for stdout)")
	pf.BoolVar(&f.noColor, "no-color", false,
		"disable ANSI color even at a terminal")
}

// validate checks flag values that can be rejected without knowing the
// destination, so a bad --format fails before any network call (and before it
// spends a metered query). Auto-detection with an empty --format is always
// valid and is resolved later, per destination, in resolveFormat.
func (f *outputFlags) validate() error {
	if f.format == "" {
		return nil
	}
	_, err := output.ParseFormat(f.format)
	return err
}

// emit resolves the destination, format, and color from the flags and renders
// v through the output package.
func (f *outputFlags) emit(cmd *cobra.Command, v any) error {
	w, closeOut, err := f.writer(cmd)
	if err != nil {
		return err
	}
	defer func() { _ = closeOut() }()

	format, err := f.resolveFormat(w)
	if err != nil {
		return err
	}

	// Color is a property of the resolved destination: only a terminal that the
	// user has not opted out of gets ANSI. A file or pipe never does.
	color := output.IsTerminal(w) && !f.noColor
	return output.Render(w, format, color, v)
}

// writer returns the destination and a close function. "-" (or unset) writes to
// the command's stdout; any other value is opened as a file to create/truncate.
func (f *outputFlags) writer(cmd *cobra.Command) (io.Writer, func() error, error) {
	if f.output == "" || f.output == "-" {
		return cmd.OutOrStdout(), func() error { return nil }, nil
	}
	file, err := os.Create(f.output)
	if err != nil {
		return nil, nil, fmt.Errorf("create output file %q: %w", f.output, err)
	}
	return file, file.Close, nil
}

// resolveFormat returns the explicit --format if set, otherwise the TTY-aware
// default for the destination w.
func (f *outputFlags) resolveFormat(w io.Writer) (output.Format, error) {
	if f.format == "" {
		return output.DefaultFormat(output.IsTerminal(w)), nil
	}
	return output.ParseFormat(f.format)
}
