package output

import (
	"bytes"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/spurintel/cli/internal/spur"
)

// TestRenderTextWrapsLongValues renders a tag whose description far exceeds
// report width and checks that continuation lines stay within 80 columns
// and align beneath the value column.
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
		if w := lipgloss.Width(line); w > 80 {
			t.Errorf("line exceeds the 80-column report width (%d): %q", w, line)
		}
		if strings.Contains(line, "residential proxy network") {
			descLines++
			if !strings.HasPrefix(line, "Description         ") && !strings.HasPrefix(line, strings.Repeat(" ", 20)) {
				t.Errorf("wrapped description line lost its value alignment: %q", line)
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

func TestReportsShareLayout(t *testing.T) {
	for name, value := range map[string]any{
		"context": sampleContext(),
		"status":  spur.Status{Active: true, QueriesRemaining: 42},
		"tag":     spur.TagMetadata{Tag: "EXAMPLE_PROXY", Name: "Example"},
		"feed":    spur.FeedMetadata{Type: "anonymous", JSON: spur.FeedFile{Date: "2026-10-02"}},
		"catalog": spur.FeedTypes,
	} {
		t.Run(name, func(t *testing.T) {
			var b bytes.Buffer
			if err := Render(&b, FormatText, false, value); err != nil {
				t.Fatal(err)
			}
			out := b.String()
			if strings.Contains(out, "SPUR / ") || strings.HasPrefix(out, "\n") {
				t.Fatalf("redundant header or leading blank line: %s", out)
			}
			if strings.ContainsAny(out, "◆├└│") {
				t.Fatalf("legacy tree formatting: %s", out)
			}
			for _, line := range strings.Split(out, "\n") {
				if lipgloss.Width(line) > 80 {
					t.Errorf("overlong line: %q", line)
				}
			}
			if name != "catalog" && !strings.Contains(out, "None") {
				t.Errorf("empty value placeholder missing: %s", out)
			}
		})
	}
}

func TestReportRowsNarrowWidthAndLongLabels(t *testing.T) {
	for _, width := range []int{24, 39, 40, 56, 80} {
		var b strings.Builder
		s := NewStyles(&bytes.Buffer{}, false)
		for _, key := range []string{"Description", "anonymous-residential/realtime"} {
			writeReportRow(&b, s, width, key, strings.Repeat("a long value ", 20))
		}
		for _, line := range strings.Split(b.String(), "\n") {
			if lipgloss.Width(line) > width {
				t.Errorf("width %d exceeded: %q", width, line)
			}
		}
	}
}
