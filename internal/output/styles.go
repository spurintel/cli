package output

import (
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Styles is the palette for the text renderer, bound to a specific writer.
//
// Color is decided by the caller, not auto-detected: color==true forces a
// TrueColor profile, color==false forces Ascii (no ANSI at all). This keeps
// the --no-color / non-terminal decision in one place (the command layer) and
// makes styled output deterministic in tests, which write to a buffer.
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

// newStyles builds a palette that renders to w. When color is false every
// style degrades to plain text via the Ascii profile.
func newStyles(w io.Writer, color bool) Styles {
	r := lipgloss.NewRenderer(w)
	if color {
		r.SetColorProfile(termenv.TrueColor)
	} else {
		r.SetColorProfile(termenv.Ascii)
	}

	accent := lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#A277FF"}
	secondary := lipgloss.AdaptiveColor{Light: "#047857", Dark: "#61FFCA"}
	warm := lipgloss.AdaptiveColor{Light: "#B45309", Dark: "#FFCA85"}
	muted := lipgloss.AdaptiveColor{Light: "#6B7280", Dark: "#9A94A8"}
	foreground := lipgloss.AdaptiveColor{Light: "#111827", Dark: "#EDECEE"}

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
