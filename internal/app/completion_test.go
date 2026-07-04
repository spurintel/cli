package app

import (
	"bytes"
	"strings"
	"testing"
)

// TestCompletionEmitsScriptPerShell checks that each supported shell produces a
// non-empty script carrying the shell's recognizable header and the `__complete`
// hook that gives live subcommand/flag completion once sourced.
func TestCompletionEmitsScriptPerShell(t *testing.T) {
	t.Parallel()
	tests := []struct {
		shell  string
		marker string // a signature unique to a valid script for this shell
	}{
		{"bash", "# bash completion V2 for spur"},
		{"zsh", "#compdef spur"},
		{"fish", "complete -c spur"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.shell, func(t *testing.T) {
			t.Parallel()
			var out bytes.Buffer
			root := newRootCmd()
			root.SetOut(&out)
			root.SetErr(&out)
			root.SetArgs([]string{"completion", tt.shell})
			if err := root.Execute(); err != nil {
				t.Fatalf("completion %s: %v", tt.shell, err)
			}
			got := out.String()
			if !strings.Contains(got, tt.marker) {
				t.Errorf("completion %s missing marker %q; script begins:\n%.120s", tt.shell, tt.marker, got)
			}
			if !strings.Contains(got, "__complete") {
				t.Errorf("completion %s does not wire the __complete hook, so tab-completion would be dead", tt.shell)
			}
		})
	}
}

// TestCompletionRejectsUnknownShell guards the ValidArgs constraint: an
// unsupported shell (e.g. powershell) must fail rather than emit a bad script.
func TestCompletionRejectsUnknownShell(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"completion", "powershell"})
	if err := root.Execute(); err == nil {
		t.Error("completion powershell = nil error, want rejection of unsupported shell")
	}
}

// TestCompletionRequiresExactlyOneShell rejects a bare `completion` invocation.
func TestCompletionRequiresExactlyOneShell(t *testing.T) {
	t.Parallel()
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"completion"})
	if err := root.Execute(); err == nil {
		t.Error("bare completion = nil error, want an arg-count error")
	}
}
