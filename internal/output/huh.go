package output

import (
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// HuhTheme is the form theme for interactive prompts (spur auth), derived from
// the same palette as the report renderer so the prompt and the output read as
// one product. When color is false the base theme is returned unchanged, which
// renders without color.
func HuhTheme(color bool) *huh.Theme {
	t := huh.ThemeBase()
	if !color {
		return t
	}

	t.Focused.Title = t.Focused.Title.Foreground(accent).Bold(true)
	t.Focused.Description = t.Focused.Description.Foreground(muted)
	t.Focused.Base = t.Focused.Base.BorderForeground(accent)
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(warm)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(warm)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(accent)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(secondary)
	t.Focused.FocusedButton = t.Focused.FocusedButton.
		Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#111827"}).
		Background(accent)
	t.Focused.BlurredButton = t.Focused.BlurredButton.
		Foreground(foreground).
		Background(muted)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Title = t.Blurred.Title.Foreground(muted).Bold(false)
	return t
}
