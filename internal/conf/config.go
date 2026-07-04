// Package conf loads the user's TOML configuration file — endpoint overrides
// and named profiles — from the config directory. It is deliberately free of
// any dependency on the client package: callers pass their built-in defaults
// into Resolve, so conf never needs to know the production hosts.
//
// Absent a config file, everything falls back to those defaults, so the CLI is
// zero-config out of the box. A profile lets a user point spur at a non-prod
// environment without touching code.
package conf

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// Config is the on-disk configuration, parsed from TOML.
type Config struct {
	// Token is the API token used when neither SPUR_TOKEN nor a selected
	// profile supplies one. It is the config-file tier the token resolver
	// consults between the environment variable and the OS keychain.
	Token string `toml:"token"`
	// Endpoints are the top-level overrides, applied over the built-in
	// defaults for every invocation regardless of profile.
	Endpoints EndpointOverrides `toml:"endpoints"`
	// Profiles are named override sets. A selected profile's endpoints are
	// applied on top of the top-level ones.
	Profiles map[string]Profile `toml:"profiles"`
}

// EndpointOverrides holds base-URL overrides. A zero-value field means "leave
// the incoming default untouched", so empty or missing sections are no-ops.
type EndpointOverrides struct {
	// API overrides the base URL of the Spur Context API.
	API string `toml:"api"`
	// Feeds overrides the base URL of the Spur Data Feeds API.
	Feeds string `toml:"feeds"`
	// Exports overrides the base URL of the Spur Exports API.
	Exports string `toml:"exports"`
}

// Profile is a named set of overrides selected with --profile.
type Profile struct {
	// Token overrides the top-level token when this profile is selected, so a
	// user can keep a distinct token per environment.
	Token     string            `toml:"token"`
	Endpoints EndpointOverrides `toml:"endpoints"`
}

// Resolved is the effective configuration after merging defaults, top-level
// overrides, and the selected profile.
type Resolved struct {
	// BaseURL is the base the shared client should talk to.
	BaseURL string
	// FeedsBaseURL is the base the shared client should use for feed requests.
	FeedsBaseURL string
	// ExportsBaseURL is the base the shared client should use for export
	// requests.
	ExportsBaseURL string
	// Token is the config-file API token, empty when the config supplies none.
	Token string
}

// DefaultPath returns the path to the default config file:
// $XDG_CONFIG_HOME/spur/config.toml, falling back to ~/.config/spur/config.toml.
func DefaultPath() (string, error) {
	if dir := os.Getenv("XDG_CONFIG_HOME"); dir != "" {
		return filepath.Join(dir, "spur", "config.toml"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("locate config directory: %w", err)
	}
	return filepath.Join(home, ".config", "spur", "config.toml"), nil
}

// Load reads and parses the TOML config at path. A missing or unreadable file
// is an error: use Load for an explicitly-requested path (--config), where a
// typo should fail loudly rather than silently fall through to defaults.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}
	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	return c, nil
}

// LoadDefault loads from DefaultPath, tolerating a missing file by returning a
// zero Config. This is the zero-config path: no file means production defaults.
func LoadDefault() (Config, error) {
	path, err := DefaultPath()
	if err != nil {
		return Config{}, err
	}
	c, err := Load(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Config{}, nil
	}
	return c, err
}

// Resolve merges the config over defaults and returns the effective settings.
//
// Precedence, lowest to highest: defaults → top-level (token + [endpoints]) →
// the selected profile's (token + [endpoints]). Empty override fields are
// skipped, so empty or absent sections change nothing. An empty profile name
// selects no profile; an unknown profile name is an error that lists the
// available profiles.
func (c Config) Resolve(profile string, defaults Resolved) (Resolved, error) {
	r := defaults
	apply := func(token string, e EndpointOverrides) {
		if token != "" {
			r.Token = token
		}
		if e.API != "" {
			r.BaseURL = e.API
		}
		if e.Feeds != "" {
			r.FeedsBaseURL = e.Feeds
		}
		if e.Exports != "" {
			r.ExportsBaseURL = e.Exports
		}
	}

	apply(c.Token, c.Endpoints)

	if profile != "" {
		p, ok := c.Profiles[profile]
		if !ok {
			return Resolved{}, fmt.Errorf("unknown profile %q; %s", profile, availableProfiles(c.Profiles))
		}
		apply(p.Token, p.Endpoints)
	}

	return r, nil
}

// availableProfiles renders a human-readable hint for an unknown-profile error.
func availableProfiles(profiles map[string]Profile) string {
	if len(profiles) == 0 {
		return "no profiles are defined in the config"
	}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return "available profiles: " + strings.Join(names, ", ")
}
