package auth

import (
	"testing"

	"github.com/zalando/go-keyring"
)

func TestStoreRejectsEmptyToken(t *testing.T) {
	kr := &fakeKeyring{}
	if err := Store(kr, ""); err == nil {
		t.Fatal("Store(empty) = nil error, want refusal")
	}
	if kr.present {
		t.Error("empty token must not be written to the keychain")
	}
}

func TestStoreThenLogoutRoundTrip(t *testing.T) {
	kr := &fakeKeyring{}
	if err := Store(kr, "tok"); err != nil {
		t.Fatalf("Store: %v", err)
	}
	if got, _ := kr.Get(); got != "tok" {
		t.Fatalf("after Store, Get = %q, want tok", got)
	}
	if err := Logout(kr); err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if got, _ := kr.Get(); got != "" {
		t.Errorf("after Logout, Get = %q, want empty", got)
	}
}

// TestOSKeyringMissingEntryIsEmptyNotError proves the production wrapper maps a
// not-found keychain entry to ("", nil) so the resolver falls through cleanly
// instead of surfacing a keychain error. go-keyring's in-memory mock stands in
// for the real OS keychain.
func TestOSKeyringMissingEntryIsEmptyNotError(t *testing.T) {
	keyring.MockInit()
	kr := DefaultKeyring()

	got, err := kr.Get()
	if err != nil {
		t.Fatalf("Get on empty keychain = %v, want nil", err)
	}
	if got != "" {
		t.Errorf("Get on empty keychain = %q, want empty", got)
	}

	// Deleting an absent entry is a no-op success.
	if err := kr.Delete(); err != nil {
		t.Fatalf("Delete on empty keychain = %v, want nil", err)
	}
}

func TestOSKeyringSetGetDelete(t *testing.T) {
	keyring.MockInit()
	kr := DefaultKeyring()

	if err := kr.Set("stored-token"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := kr.Get()
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "stored-token" {
		t.Errorf("Get = %q, want stored-token", got)
	}
	if err := kr.Delete(); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if got, _ := kr.Get(); got != "" {
		t.Errorf("after Delete, Get = %q, want empty", got)
	}
}
