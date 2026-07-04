package output

import (
	"fmt"
	"io"
)

// Status-line helpers: one-line confirmations, notices, and warnings that sit
// outside the Render formats (auth confirmations, config warnings). They share
// the report palette so every line the CLI prints reads as one product. color
// follows the same contract as Render: the caller decides it, typically
// ColorEnabled(w) && !--no-color.

// Successf writes a green-marked confirmation line to w.
func Successf(w io.Writer, color bool, format string, a ...any) {
	s := newStyles(w, color)
	fmt.Fprintf(Writer(w, color), "%s %s\n", s.Success.Render("●"), s.Value.Render(fmt.Sprintf(format, a...)))
}

// Infof writes a neutral notice line to w.
func Infof(w io.Writer, color bool, format string, a ...any) {
	s := newStyles(w, color)
	fmt.Fprintf(Writer(w, color), "%s %s\n", s.Frame.Render("●"), s.Value.Render(fmt.Sprintf(format, a...)))
}

// Warnf writes a warning line to w, marked with the cautionary palette.
func Warnf(w io.Writer, color bool, format string, a ...any) {
	s := newStyles(w, color)
	fmt.Fprintf(Writer(w, color), "%s %s\n", s.Warning.Render("▲"), s.Warning.Render(fmt.Sprintf(format, a...)))
}
