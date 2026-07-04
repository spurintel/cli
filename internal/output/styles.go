package output

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// The palette, shared by the text renderer, the status-line helpers, and the
// huh prompt theme so every surface of the CLI reads as one product. Each
// color adapts to the terminal background (light/dark).
var (
	accent     = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#A277FF"}
	secondary  = lipgloss.AdaptiveColor{Light: "#047857", Dark: "#61FFCA"}
	warm       = lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FFCA85"}
	muted      = lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9A94A8"}
	foreground = lipgloss.AdaptiveColor{Light: "#111827", Dark: "#EDECEE"}
)

// Styles is the palette for the text renderer, bound to a specific writer.
type Styles struct {
	Title   lipgloss.Style // section/report header text
	Frame   lipgloss.Style // tree connectors (├ │ └)
	Block   lipgloss.Style // block name
	Key     lipgloss.Style // field label
	Value   lipgloss.Style // field value
	Empty   lipgloss.Style // placeholder for absent values
	Bool    lipgloss.Style // boolean values
	Num     lipgloss.Style // numeric values
	Success lipgloss.Style // affirmative status markers
	Warning lipgloss.Style // cautionary status markers
}

// NewStyles builds a palette that renders to w. When color is false every
// style degrades to plain text via the Ascii profile.
//
// When color is true the profile is detected from w and the environment, so a
// 256-color terminal gets downsampled colors rather than raw 24-bit sequences
// it cannot render. Detection needs a real terminal; when w has no detectable
// capability but the caller still asked for color (tests writing to a buffer),
// TrueColor is assumed.
func NewStyles(w io.Writer, color bool) Styles {
	r := lipgloss.NewRenderer(w)
	if !color {
		r.SetColorProfile(termenv.Ascii)
		// With styling off the light/dark variant is irrelevant; pinning it
		// skips termenv's background query (an OSC 11 round-trip to the
		// terminal) that adaptive colors would otherwise trigger.
		r.SetHasDarkBackground(true)
	} else if r.ColorProfile() == termenv.Ascii {
		r.SetColorProfile(termenv.TrueColor)
	}

	return Styles{
		Title:   r.NewStyle().Bold(true).Foreground(accent),
		Frame:   r.NewStyle().Foreground(accent),
		Block:   r.NewStyle().Bold(true).Foreground(warm),
		Key:     r.NewStyle().Foreground(secondary),
		Value:   r.NewStyle().Foreground(foreground),
		Empty:   r.NewStyle().Foreground(muted).Italic(true),
		Bool:    r.NewStyle().Foreground(warm),
		Num:     r.NewStyle().Foreground(warm),
		Success: r.NewStyle().Foreground(secondary),
		Warning: r.NewStyle().Foreground(warm),
	}
}

// newStyles is the internal spelling used by the render paths.
func newStyles(w io.Writer, color bool) Styles {
	return NewStyles(w, color)
}

// ColorEnabled reports whether w should receive ANSI color: an interactive
// terminal whose environment permits it. This delegates to termenv's profile
// detection, which honors the NO_COLOR convention (https://no-color.org),
// CLICOLOR=0, and CLICOLOR_FORCE, and returns no color for a pipe or file.
// The --no-color flag is applied by the caller on top of this.
func ColorEnabled(w io.Writer) bool {
	return lipgloss.NewRenderer(w).ColorProfile() != termenv.Ascii
}
