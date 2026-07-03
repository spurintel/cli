package app

import (
	"context"
	"io"
	"log/slog"
)

// newLogger returns an slog.Logger that writes JSON with GCP Cloud Logging's
// expected field names: "severity", "message", "time".
func newLogger(w io.Writer) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     slog.LevelInfo,
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

// LoggerFromContext returns a logger from ctx, or slog.Default() if none is set.
func LoggerFromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

// WithLogger attaches a logger to ctx.
func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

type loggerKey struct{}
