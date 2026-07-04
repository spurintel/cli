package output

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/spurintel/cli/internal/spur"
)

// TestRenderTextWrapsLongValues renders a tag whose description far exceeds
// the fallback width (100 columns for a non-terminal writer) and checks that
// the value wraps onto continuation lines that stay inside the width and keep
// the tree-row prefix, instead of one overlong line breaking the layout.
func TestRenderTextWrapsLongValues(t *testing.T) {
	t.Parallel()
	long := strings.TrimSpace(strings.Repeat("residential proxy network with global exit pools ", 6))
	tag := spur.TagMetadata{Tag: "ACME_PROXY", Name: "Acme", Description: long}

	var buf bytes.Buffer
	if err := Render(&buf, FormatText, false, tag); err != nil {
		t.Fatalf("Render text: %v", err)
	}
	out := buf.String()

	descLines := 0
	for _, line := range strings.Split(out, "\n") {
		if w := lipgloss.Width(line); w > 100 {
			t.Errorf("line exceeds the 100-column fallback width (%d): %q", w, line)
		}
		if strings.Contains(line, "residential proxy network") {
			descLines++
			if !strings.HasPrefix(line, "│") {
				t.Errorf("wrapped description line lost its tree prefix: %q", line)
			}
		}
	}
	if descLines < 2 {
		t.Errorf("long description should wrap onto multiple lines, got %d:\n%s", descLines, out)
	}
}

func TestStatusLineHelpersPlain(t *testing.T) {
	t.Parallel()
	for name, f := range map[string]func(w *bytes.Buffer){
		"Successf": func(w *bytes.Buffer) { Successf(w, false, "stored %s", "token") },
		"Infof":    func(w *bytes.Buffer) { Infof(w, false, "stored %s", "token") },
		"Warnf":    func(w *bytes.Buffer) { Warnf(w, false, "stored %s", "token") },
	} {
		var buf bytes.Buffer
		f(&buf)
		out := buf.String()
		if strings.Contains(out, esc) {
			t.Errorf("%s without color contains ANSI escape codes: %q", name, out)
		}
		if !strings.Contains(out, "stored token") {
			t.Errorf("%s output missing message: %q", name, out)
		}
		if !strings.HasSuffix(out, "\n") {
			t.Errorf("%s output not newline-terminated: %q", name, out)
		}
	}
}

func TestStatusLineHelpersColor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	Warnf(&buf, true, "config is world-readable")
	if !strings.Contains(buf.String(), esc) {
		t.Errorf("colored Warnf has no ANSI escape codes: %q", buf.String())
	}
}

// TestColorEnabledNonTerminal pins the color contract for pipes and buffers:
// no ANSI unless the environment forces it.
func TestColorEnabledNonTerminal(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("CLICOLOR_FORCE", "")
	if ColorEnabled(&bytes.Buffer{}) {
		t.Error("ColorEnabled(buffer) = true, want false")
	}
}

func TestHumanizeBytes(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		n    int64
		want string
	}{
		{512, "512 B"},
		{2048, "2.0 KiB"},
		{1288490189, "1.2 GiB"},
	} {
		if got := HumanizeBytes(tc.n); got != tc.want {
			t.Errorf("HumanizeBytes(%d) = %q, want %q", tc.n, got, tc.want)
		}
	}
}
