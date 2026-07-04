package auth

import (
	"errors"
	"testing"
)

// fakeKeyring is an in-memory Keyring for exercising the resolver and the auth
// command without touching the real OS keychain.
type fakeKeyring struct {
	token   string
	present bool
	getErr  error
	setErr  error
	delErr  error
}

func (f *fakeKeyring) Get() (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	if !f.present {
		return "", nil
	}
	return f.token, nil
}

func (f *fakeKeyring) Set(token string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.token, f.present = token, true
	return nil
}

func (f *fakeKeyring) Delete() error {
	if f.delErr != nil {
		return f.delErr
	}
	f.token, f.present = "", false
	return nil
}

func TestResolvePrecedence(t *testing.T) {
	cases := []struct {
		name     string
		env      string
		cfg      string
		keychain string // "" means the keychain has no stored token
		want     string
	}{
		{"env over all", "env-tok", "cfg-tok", "kc-tok", "env-tok"},
		{"env over config", "env-tok", "cfg-tok", "", "env-tok"},
		{"config over keychain", "", "cfg-tok", "kc-tok", "cfg-tok"},
		{"keychain when only source", "", "", "kc-tok", "kc-tok"},
		{"env alone", "env-tok", "", "", "env-tok"},
		{"config alone", "", "cfg-tok", "", "cfg-tok"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			kr := &fakeKeyring{token: tc.keychain, present: tc.keychain != ""}
			got, err := Resolve(tc.env, tc.cfg, kr)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != tc.want {
				t.Errorf("Resolve(%q,%q,kc=%q) = %q, want %q", tc.env, tc.cfg, tc.keychain, got, tc.want)
			}
		})
	}
}

func TestResolveAllEmptyReturnsErrNoToken(t *testing.T) {
	_, err := Resolve("", "", &fakeKeyring{})
	if !errors.Is(err, ErrNoToken) {
		t.Fatalf("Resolve with no sources = %v, want ErrNoToken", err)
	}
}

func TestResolveHigherTierSkipsKeychainRead(t *testing.T) {
	// A hit in the env or config tier must not touch the keychain, so a keychain
	// that would error never gets consulted.
	kr := &fakeKeyring{getErr: errors.New("keychain must not be read")}
	got, err := Resolve("env-tok", "", kr)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != "env-tok" {
		t.Errorf("Resolve = %q, want env-tok", got)
	}
}

func TestResolveKeychainErrorPropagates(t *testing.T) {
	kr := &fakeKeyring{getErr: errors.New("boom")}
	if _, err := Resolve("", "", kr); err == nil {
		t.Fatal("Resolve with keychain error = nil, want error")
	}
}
