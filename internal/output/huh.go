package output

import (
	"charm.land/huh/v2"
	"charm.land/lipgloss/v2"
)

// HuhTheme is the form theme for interactive prompts (spur auth), derived from
// the same palette as the report renderer so the prompt and the output read as
// one product. huh detects the terminal background itself and passes isDark.
// When color is false the base theme is returned unchanged, which renders
// without color.
func HuhTheme(color bool) huh.Theme {
	return huh.ThemeFunc(func(isDark bool) *huh.Styles {
		t := huh.ThemeBase(isDark)
		if !color {
			return t
		}
		p := newPalette(isDark)
		pick := lipgloss.LightDark(isDark)

		t.Focused.Title = t.Focused.Title.Foreground(p.accent).Bold(true)
		t.Focused.Description = t.Focused.Description.Foreground(p.muted)
		t.Focused.Base = t.Focused.Base.BorderForeground(p.accent)
		t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(p.warm)
		t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(p.warm)
		t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(p.accent)
		t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(p.secondary)
		t.Focused.FocusedButton = t.Focused.FocusedButton.
			Foreground(pick(lipgloss.Color("#FFFFFF"), lipgloss.Color("#111827"))).
			Background(p.accent)
		t.Focused.BlurredButton = t.Focused.BlurredButton.
			Foreground(p.foreground).
			Background(p.muted)

		t.Blurred = t.Focused
		t.Blurred.Base = t.Blurred.Base.BorderStyle(lipgloss.HiddenBorder())
		t.Blurred.Title = t.Blurred.Title.Foreground(p.muted).Bold(false)
		return t
	})
}
