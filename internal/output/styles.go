package output

import (
	"image/color"
	"io"
	"os"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"golang.org/x/term"
)

// palette is the CLI's color vocabulary, resolved for a light or dark
// terminal background. It is shared by the text renderer, the status-line
// helpers, and the huh prompt theme so every surface reads as one product.
type palette struct {
	accent     color.Color
	secondary  color.Color
	warm       color.Color
	muted      color.Color
	foreground color.Color
}

// newPalette resolves the palette for the given background.
func newPalette(isDark bool) palette {
	pick := lipgloss.LightDark(isDark)
	return palette{
		accent:     pick(lipgloss.Color("#4C6500"), lipgloss.Color("#D7FF44")),
		secondary:  pick(lipgloss.Color("#047857"), lipgloss.Color("#61FFCA")),
		warm:       pick(lipgloss.Color("#B45309"), lipgloss.Color("#FFCA85")),
		muted:      pick(lipgloss.Color("#6B7280"), lipgloss.Color("#9A94A8")),
		foreground: pick(lipgloss.Color("#111827"), lipgloss.Color("#EDECEE")),
	}
}

// Styles is the palette for the text renderer.
//
// In lipgloss v2 styles are not bound to a writer: they always render full
// ANSI, and Writer degrades that output to what the destination supports
// (including stripping it entirely). Always print rendered styles through
// Writer(w, color).
type Styles struct {
	Title   lipgloss.Style // section/report header text
	Frame   lipgloss.Style // progress bar accent
	Value   lipgloss.Style // field value
	Empty   lipgloss.Style // placeholder for absent values
	Bool    lipgloss.Style // boolean values
	Num     lipgloss.Style // numeric values
	Success lipgloss.Style // affirmative status markers
	Warning lipgloss.Style // cautionary status markers
}

// NewStyles builds the style palette for output destined for w. w is used
// only to pick the light or dark color variants: when it is a color-enabled
// terminal its background is queried; otherwise the dark palette is assumed,
// including when color is explicitly forced for a non-terminal writer.
func NewStyles(w io.Writer, color bool) Styles {
	dark := true
	if color {
		if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
			dark = lipgloss.HasDarkBackground(os.Stdin, f)
		}
	}
	p := newPalette(dark)

	return Styles{
		Title:   lipgloss.NewStyle().Bold(true).Foreground(p.accent),
		Frame:   lipgloss.NewStyle().Foreground(p.accent),
		Value:   lipgloss.NewStyle().Foreground(p.foreground),
		Empty:   lipgloss.NewStyle().Foreground(p.muted),
		Bool:    lipgloss.NewStyle().Foreground(p.warm),
		Num:     lipgloss.NewStyle().Foreground(p.warm),
		Success: lipgloss.NewStyle().Foreground(p.secondary),
		Warning: lipgloss.NewStyle().Foreground(p.warm),
	}
}

// newStyles is the internal spelling used by the render paths.
func newStyles(w io.Writer, color bool) Styles {
	return NewStyles(w, color)
}

// Writer wraps w so that styled output is degraded to what the destination
// supports: full color passes through, a 256-color terminal gets downsampled
// codes, and color==false strips ANSI entirely. When the caller demands color
// on a writer with no detectable capability (tests writing to a buffer),
// TrueColor is assumed.
func Writer(w io.Writer, color bool) io.Writer {
	cw := colorprofile.NewWriter(w, os.Environ())
	if !color {
		cw.Profile = colorprofile.NoTTY
	} else if cw.Profile <= colorprofile.Ascii {
		cw.Profile = colorprofile.TrueColor
	}
	return cw
}

// ColorEnabled reports whether the destination and environment permit ANSI
// color. Detection honors NO_COLOR, CLICOLOR=0, and CLICOLOR_FORCE; pipes and
// files are uncolored unless color is forced. The caller applies --no-color.
func ColorEnabled(w io.Writer) bool {
	return colorprofile.Detect(w, os.Environ()) > colorprofile.Ascii
}
