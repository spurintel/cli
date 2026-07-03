// Package output is the CLI's single rendering entry point. Render takes a
// format and a value and writes it as human-styled text, newline-delimited
// JSON, or CSV. It is a deep module: callers decide the format and whether
// color is wanted, and the package hides lipgloss styling, per-entity layout,
// and the tolerant CSV/JSON encoding behind one call.
//
// The package imports the spur response types it renders; spur stays a pure
// API client with no presentation dependency, so the direction is output →
// spur only.
package output

import (
	"encoding/json"
	"fmt"
	"io"

	"golang.org/x/term"

	"github.com/spurintel/cli/internal/spur"
)

// Format is a supported output encoding.
type Format string

const (
	// FormatText is the human-readable, styled report shown at a terminal.
	FormatText Format = "text"
	// FormatJSON is newline-delimited JSON, the pipe-friendly default.
	FormatJSON Format = "json"
	// FormatCSV is a header row plus one record per rendered value.
	FormatCSV Format = "csv"
)

// formats is the canonical list of valid --format values, for validation and
// help text.
var formats = []Format{FormatText, FormatJSON, FormatCSV}

// ParseFormat validates a --format flag value.
func ParseFormat(s string) (Format, error) {
	f := Format(s)
	for _, valid := range formats {
		if f == valid {
			return f, nil
		}
	}
	return "", fmt.Errorf("invalid output format %q; valid formats: %s", s, FormatList())
}

// FormatList is the pipe-joined list of valid formats, for flag help.
func FormatList() string {
	s := ""
	for i, f := range formats {
		if i > 0 {
			s += "|"
		}
		s += string(f)
	}
	return s
}

// DefaultFormat is the format used when the user did not pass --format. A
// terminal gets the styled human report; anything piped or redirected gets
// machine-friendly JSON so existing pipelines keep working.
func DefaultFormat(isTTY bool) Format {
	if isTTY {
		return FormatText
	}
	return FormatJSON
}

// IsTerminal reports whether w is an interactive terminal. A plain io.Writer
// (a file, a buffer, a pipe) is not, so this drives both the default-format
// choice and color suppression.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// Render writes v to w in the given format. color enables ANSI styling for the
// text format; the caller is responsible for deciding it (typically terminal
// && !--no-color) so Render never second-guesses a non-terminal write. color
// is ignored for JSON and CSV, which are always plain.
func Render(w io.Writer, format Format, color bool, v any) error {
	switch format {
	case FormatJSON:
		return renderJSON(w, v)
	case FormatCSV:
		return renderCSV(w, v)
	case FormatText:
		return renderText(w, color, v)
	default:
		return fmt.Errorf("invalid output format %q; valid formats: %s", format, FormatList())
	}
}

// renderJSON writes v as newline-delimited JSON: compact, one object per line,
// so a batch pipes straight into jq and a single value is a single line. A slice
// of contexts is emitted as one object per line (NDJSON), not a JSON array, so
// the single-value and batch outputs share the same line-oriented shape.
func renderJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	if batch, ok := v.([]spur.IPContext); ok {
		for i := range batch {
			if err := enc.Encode(batch[i]); err != nil {
				return fmt.Errorf("encode json: %w", err)
			}
		}
		return nil
	}
	if err := enc.Encode(v); err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return nil
}
