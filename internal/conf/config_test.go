package conf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// prodDefaults is a stand-in for the built-in production settings the app layer
// passes in (kept local so conf has no dependency on the client package).
var prodDefaults = Resolved{BaseURL: "https://api.spur.us", FeedsBaseURL: "https://feeds.spur.us", ExportsBaseURL: "https://exports.spur.us"}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadDefaultMissingFileYieldsZeroConfig(t *testing.T) {
	// A missing default config must not error: zero-config falls back to
	// production defaults everywhere.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir()) // empty dir, no config.toml
	cfg, err := LoadDefault()
	if err != nil {
		t.Fatalf("LoadDefault with no file: %v", err)
	}
	got, err := cfg.Resolve("", prodDefaults)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.BaseURL != prodDefaults.BaseURL {
		t.Errorf("BaseURL = %q, want default %q", got.BaseURL, prodDefaults.BaseURL)
	}
}

func TestLoadStrictMissingFileErrors(t *testing.T) {
	// An explicitly-passed --config path that does not exist is a user error,
	// not a silent fall-through to defaults.
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.toml"))
	if err == nil {
		t.Fatal("Load of missing file = nil error, want error")
	}
}

func TestDefaultPathUsesXDGConfigHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/xdg")
	p, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if want := filepath.Join("/xdg", "spur", "config.toml"); p != want {
		t.Errorf("DefaultPath = %q, want %q", p, want)
	}
}

func TestTopLevelEndpointOverride(t *testing.T) {
	path := writeConfig(t, `
[endpoints]
api = "https://api.staging.spur.us"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := cfg.Resolve("", prodDefaults)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := "https://api.staging.spur.us"; got.BaseURL != want {
		t.Errorf("BaseURL = %q, want %q", got.BaseURL, want)
	}
}

func TestFeedsEndpointOverride(t *testing.T) {
	// The feeds host overrides independently of the Context API host: overriding
	// one must leave the other at its default.
	path := writeConfig(t, `
[endpoints]
feeds = "https://feeds.staging.spur.us"

[profiles.local.endpoints]
feeds = "http://localhost:9090"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := []struct {
		profile   string
		wantFeeds string
	}{
		{"", "https://feeds.staging.spur.us"},
		{"local", "http://localhost:9090"},
	}
	for _, tc := range cases {
		got, err := cfg.Resolve(tc.profile, prodDefaults)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tc.profile, err)
		}
		if got.FeedsBaseURL != tc.wantFeeds {
			t.Errorf("Resolve(%q).FeedsBaseURL = %q, want %q", tc.profile, got.FeedsBaseURL, tc.wantFeeds)
		}
		// Overriding feeds must not disturb the Context API base.
		if got.BaseURL != prodDefaults.BaseURL {
			t.Errorf("Resolve(%q).BaseURL = %q, want default %q", tc.profile, got.BaseURL, prodDefaults.BaseURL)
		}
	}
}

func TestExportsEndpointOverride(t *testing.T) {
	// The exports host overrides independently of the Context API and feeds
	// hosts: overriding one must leave the others at their defaults.
	path := writeConfig(t, `
[endpoints]
exports = "https://exports.staging.spur.us"

[profiles.local.endpoints]
exports = "http://localhost:9091"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := []struct {
		profile     string
		wantExports string
	}{
		{"", "https://exports.staging.spur.us"},
		{"local", "http://localhost:9091"},
	}
	for _, tc := range cases {
		got, err := cfg.Resolve(tc.profile, prodDefaults)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tc.profile, err)
		}
		if got.ExportsBaseURL != tc.wantExports {
			t.Errorf("Resolve(%q).ExportsBaseURL = %q, want %q", tc.profile, got.ExportsBaseURL, tc.wantExports)
		}
		// Overriding exports must not disturb the other hosts.
		if got.BaseURL != prodDefaults.BaseURL {
			t.Errorf("Resolve(%q).BaseURL = %q, want default %q", tc.profile, got.BaseURL, prodDefaults.BaseURL)
		}
		if got.FeedsBaseURL != prodDefaults.FeedsBaseURL {
			t.Errorf("Resolve(%q).FeedsBaseURL = %q, want default %q", tc.profile, got.FeedsBaseURL, prodDefaults.FeedsBaseURL)
		}
	}
}

func TestProfileOverridesTopLevel(t *testing.T) {
	path := writeConfig(t, `
[endpoints]
api = "https://api.top.spur.us"

[profiles.staging.endpoints]
api = "https://api.staging.spur.us"

[profiles.local.endpoints]
api = "http://localhost:8080"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := []struct {
		profile string
		want    string
	}{
		{"", "https://api.top.spur.us"}, // no profile: top-level wins over default
		{"staging", "https://api.staging.spur.us"},
		{"local", "http://localhost:8080"},
	}
	for _, tc := range cases {
		got, err := cfg.Resolve(tc.profile, prodDefaults)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tc.profile, err)
		}
		if got.BaseURL != tc.want {
			t.Errorf("Resolve(%q).BaseURL = %q, want %q", tc.profile, got.BaseURL, tc.want)
		}
	}
}

func TestUnknownProfileErrorsClearly(t *testing.T) {
	path := writeConfig(t, `
[profiles.staging.endpoints]
api = "https://api.staging.spur.us"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	_, err = cfg.Resolve("prod", prodDefaults)
	if err == nil {
		t.Fatal("Resolve of unknown profile = nil error, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "prod") {
		t.Errorf("error %q should name the unknown profile %q", msg, "prod")
	}
	if !strings.Contains(msg, "staging") {
		t.Errorf("error %q should list the available profile %q", msg, "staging")
	}
}

func TestEmptyAndMissingSectionsAreNoOps(t *testing.T) {
	// Empty [endpoints], a profile with no endpoints, and an empty api string
	// must all leave the incoming defaults untouched.
	path := writeConfig(t, `
[endpoints]

[profiles.bare]

[profiles.blank.endpoints]
api = ""
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	for _, profile := range []string{"", "bare", "blank"} {
		got, err := cfg.Resolve(profile, prodDefaults)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", profile, err)
		}
		if got.BaseURL != prodDefaults.BaseURL {
			t.Errorf("Resolve(%q).BaseURL = %q, want default %q", profile, got.BaseURL, prodDefaults.BaseURL)
		}
	}
}

func TestTokenResolution(t *testing.T) {
	// A token may live at the top level and be overridden per-profile, exactly
	// like an endpoint. It is the config-file tier the token resolver reads
	// between the environment variable and the keychain.
	path := writeConfig(t, `
token = "top-token"

[profiles.staging]
token = "staging-token"

[profiles.local.endpoints]
api = "http://localhost:8080"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	cases := []struct {
		profile string
		want    string
	}{
		{"", "top-token"},            // top-level token resolves
		{"staging", "staging-token"}, // profile token overrides top-level
		{"local", "top-token"},       // profile without a token inherits top-level
	}
	for _, tc := range cases {
		got, err := cfg.Resolve(tc.profile, prodDefaults)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tc.profile, err)
		}
		if got.Token != tc.want {
			t.Errorf("Resolve(%q).Token = %q, want %q", tc.profile, got.Token, tc.want)
		}
	}
}

func TestMissingTokenResolvesEmpty(t *testing.T) {
	// No token anywhere in the config must leave Resolved.Token empty so the
	// resolver falls through to the keychain rather than sending a blank token.
	path := writeConfig(t, `
[endpoints]
api = "https://api.staging.spur.us"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got, err := cfg.Resolve("", prodDefaults)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.Token != "" {
		t.Errorf("Resolve().Token = %q, want empty", got.Token)
	}
}

func TestEmptyConfigFileIsValid(t *testing.T) {
	path := writeConfig(t, "")
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load of empty file: %v", err)
	}
	got, err := cfg.Resolve("", prodDefaults)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got.BaseURL != prodDefaults.BaseURL {
		t.Errorf("BaseURL = %q, want default %q", got.BaseURL, prodDefaults.BaseURL)
	}
}
