package app

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spurintel/cli/internal/output"
)

func TestResolveFormatAutoDetects(t *testing.T) {
	t.Parallel()
	// A bytes.Buffer is not a terminal, so an unset --format must fall back to
	// JSON (the piped default).
	f := &outputFlags{}
	got, err := f.resolveFormat(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("resolveFormat: %v", err)
	}
	if got != output.FormatJSON {
		t.Errorf("auto format for non-terminal = %q, want %q", got, output.FormatJSON)
	}
}

func TestResolveFormatExplicitOverrides(t *testing.T) {
	t.Parallel()
	f := &outputFlags{format: "csv"}
	got, err := f.resolveFormat(&bytes.Buffer{})
	if err != nil {
		t.Fatalf("resolveFormat: %v", err)
	}
	if got != output.FormatCSV {
		t.Errorf("explicit format = %q, want %q", got, output.FormatCSV)
	}
}

func TestResolveFormatRejectsInvalid(t *testing.T) {
	t.Parallel()
	f := &outputFlags{format: "yaml"}
	if _, err := f.resolveFormat(&bytes.Buffer{}); err == nil {
		t.Error("resolveFormat with invalid format = nil error, want error")
	}
}

func TestValidateFailsFastOnBadFormat(t *testing.T) {
	t.Parallel()
	if err := (&outputFlags{}).validate(); err != nil {
		t.Errorf("empty format should be valid (auto-detect), got %v", err)
	}
	if err := (&outputFlags{format: "json"}).validate(); err != nil {
		t.Errorf("json should validate, got %v", err)
	}
	if err := (&outputFlags{format: "yaml"}).validate(); err == nil {
		t.Error("yaml should fail validation, got nil")
	}
}

func TestOutputWriterFile(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "out.json")
	f := &outputFlags{output: path}

	w, closeFn, err := f.writer(nil)
	if err != nil {
		t.Fatalf("writer: %v", err)
	}
	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("file contents = %q, want %q", got, "hello")
	}
}
