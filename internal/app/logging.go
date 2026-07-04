package app

import (
	"io"
	"log/slog"
	"strings"
)

// newLogger returns an slog.Logger that writes JSON with GCP Cloud Logging's
// expected field names: "severity", "message", "time".
func newLogger(w io.Writer, level slog.Level) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			switch a.Key {
			case slog.LevelKey:
				return slog.String("severity", gcpSeverity(a.Value.Any().(slog.Level)))
			case slog.MessageKey:
				a.Key = "message"
			}
			return a
		},
	})
	return slog.New(h)
}

func gcpSeverity(l slog.Level) string {
	switch {
	case l >= slog.LevelError:
		return "ERROR"
	case l >= slog.LevelWarn:
		return "WARNING"
	case l >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}

// parseLogLevel maps a LOG_LEVEL string onto an slog.Level. Unknown values
// fall back to Info rather than erroring: a typo in a diagnostic knob should
// never break the command itself.
func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
