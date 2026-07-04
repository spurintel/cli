package app

import "os"

// Config holds runtime configuration loaded from environment.
// Extend as needed; keep fields grouped by concern.
type Config struct {
	LogLevel string
}

// loadConfig reads environment variables into a Config.
func loadConfig() (Config, error) {
	return Config{
		LogLevel: envOr("LOG_LEVEL", "info"),
	}, nil
}

func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
