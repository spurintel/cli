package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spurintel/cli/internal/spur"
)

func writeConfigFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestResolveNoConfigUsesProductionDefault(t *testing.T) {
	// Point the default lookup at an empty dir so no real ~/.config file leaks
	// into the test; an absent file must resolve to the built-in base URL.
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cf := &configFlags{}
	if err := cf.resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cf.resolved.BaseURL != spur.BaseURL {
		t.Errorf("BaseURL = %q, want production default %q", cf.resolved.BaseURL, spur.BaseURL)
	}
	if got := cf.newClient("tok").Base; got != spur.BaseURL {
		t.Errorf("client Base = %q, want %q", got, spur.BaseURL)
	}
}

func TestResolveConfigOverridesBaseURL(t *testing.T) {
	path := writeConfigFile(t, `
[endpoints]
api = "https://api.staging.example.test"
`)
	cf := &configFlags{path: path}
	if err := cf.resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if want := "https://api.staging.example.test"; cf.resolved.BaseURL != want {
		t.Errorf("BaseURL = %q, want %q", cf.resolved.BaseURL, want)
	}
	if got := cf.newClient("tok").Base; got != "https://api.staging.example.test" {
		t.Errorf("client Base = %q, want override", got)
	}
}

func TestResolveProfileSelection(t *testing.T) {
	path := writeConfigFile(t, `
[profiles.staging.endpoints]
api = "https://api.staging.example.test"
`)
	cf := &configFlags{path: path, profile: "staging"}
	if err := cf.resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if want := "https://api.staging.example.test"; cf.resolved.BaseURL != want {
		t.Errorf("BaseURL = %q, want %q", cf.resolved.BaseURL, want)
	}
}

func TestResolveUnknownProfileErrors(t *testing.T) {
	path := writeConfigFile(t, `
[profiles.staging.endpoints]
api = "https://api.staging.example.test"
`)
	cf := &configFlags{path: path, profile: "nope"}
	err := cf.resolve()
	if err == nil {
		t.Fatal("resolve with unknown profile = nil error, want error")
	}
	if !strings.Contains(err.Error(), "nope") {
		t.Errorf("error %q should name the unknown profile", err.Error())
	}
}

func TestResolveMissingExplicitConfigErrors(t *testing.T) {
	cf := &configFlags{path: filepath.Join(t.TempDir(), "absent.toml")}
	if err := cf.resolve(); err == nil {
		t.Fatal("resolve with missing --config path = nil error, want error")
	}
}

// TestConfigOverrideDrivesClientToOverriddenHost is the end-to-end proof of
// acceptance criterion 2: a config endpoint override makes the client actually
// call the overridden host. A test server stands in for the override target.
func TestConfigOverrideDrivesClientToOverriddenHost(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ip":"8.8.8.8","infrastructure":"DATACENTER"}`))
	}))
	defer srv.Close()

	path := writeConfigFile(t, "[endpoints]\napi = \""+srv.URL+"\"\n")
	cf := &configFlags{path: path}
	if err := cf.resolve(); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	client := cf.newClient("tok")
	got, err := fetchContext(context.Background(), client, "8.8.8.8", spur.IPContextOptions{})
	if err != nil {
		t.Fatalf("fetchContext against override host: %v", err)
	}
	if !hit {
		t.Fatal("override host was never called")
	}
	if got.IP != "8.8.8.8" {
		t.Errorf("fetched IP = %q, want 8.8.8.8", got.IP)
	}
}
