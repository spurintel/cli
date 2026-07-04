package app

import (
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/spurintel/cli/internal/auth"
)

// TestResolveTokenPrecedence exercises the app-layer wiring end-to-end: env
// (SPUR_TOKEN) → config-file token → OS keychain. go-keyring's in-memory mock
// stands in for the real keychain so the test controls the lowest tier.
func TestResolveTokenPrecedence(t *testing.T) {
	cases := []struct {
		name     string
		env      string
		cfg      string
		keychain string // "" means nothing stored in the keychain
		want     string
	}{
		{"env beats config and keychain", "env-tok", "cfg-tok", "kc-tok", "env-tok"},
		{"config beats keychain", "", "cfg-tok", "kc-tok", "cfg-tok"},
		{"keychain when env and config empty", "", "", "kc-tok", "kc-tok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			keyring.MockInit()
			if tc.keychain != "" {
				if err := auth.DefaultKeyring().Set(tc.keychain); err != nil {
					t.Fatalf("seed keychain: %v", err)
				}
			}
			t.Setenv(auth.EnvVar, tc.env)

			got, err := resolveToken(tc.cfg)
			if err != nil {
				t.Fatalf("resolveToken: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveToken(env=%q,cfg=%q,kc=%q) = %q, want %q",
					tc.env, tc.cfg, tc.keychain, got, tc.want)
			}
		})
	}
}

// TestResolveTokenNoSourcesIsActionable proves a total miss yields a clear,
// actionable error naming every way to supply a token — not a bare keychain
// error or the raw ErrNoToken sentinel.
func TestResolveTokenNoSourcesIsActionable(t *testing.T) {
	keyring.MockInit()
	t.Setenv(auth.EnvVar, "")

	_, err := resolveToken("")
	if err == nil {
		t.Fatal("resolveToken with no sources = nil error, want actionable error")
	}
	msg := err.Error()
	for _, want := range []string{"SPUR_TOKEN", "spur auth", "config"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should mention %q", msg, want)
		}
	}
}
