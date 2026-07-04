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
		"disable ANSI color even at a terminal (the NO_COLOR environment variable is also honored)")
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
// v through the output package. A close failure on a -o file (a deferred flush
// hitting a full disk) surfaces as the command error rather than being
// swallowed, since it means the written output is incomplete.
func (f *outputFlags) emit(cmd *cobra.Command, v any) (err error) {
	w, closeOut, err := f.writer(cmd)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := closeOut(); cerr != nil && err == nil {
			err = fmt.Errorf("close output file %q: %w", f.output, cerr)
		}
	}()

	format, err := f.resolveFormat(w)
	if err != nil {
		return err
	}

	return output.Render(w, format, f.colorEnabled(w), v)
}

// colorEnabled reports whether the destination w should receive ANSI color:
// a color-capable terminal (or a CLICOLOR_FORCE-d environment) that the user
// has not opted out of via --no-color or NO_COLOR. A file or pipe never gets
// color unless forced.
func (f *outputFlags) colorEnabled(w io.Writer) bool {
	return !f.noColor && output.ColorEnabled(w)
}

// toStdout reports whether the resolved destination is stdout rather than a
// file.
func (f *outputFlags) toStdout() bool {
	return f.output == "" || f.output == "-"
}

// sink returns a function that opens the destination on first use. The
// streaming commands (feed download, export) take it instead of an opened
// writer so a -o file is only created — and a pre-existing file only
// truncated — after the API request has succeeded; opening up front would
// destroy the file on an auth failure or rate limit.
func (f *outputFlags) sink(cmd *cobra.Command) func() (io.Writer, func() error, error) {
	return func() (io.Writer, func() error, error) {
		return f.writer(cmd)
	}
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
