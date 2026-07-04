package auth

import (
	"errors"
	"fmt"

	"github.com/zalando/go-keyring"
)

// Keychain identifiers for the stored token. service is the label the OS
// keychain shows for the item; account distinguishes it within the service.
// A single token is stored per machine (profiles select tokens from the config
// file, not the keychain), so a fixed account is sufficient.
const (
	keychainService = "spur"
	keychainAccount = "api-token"
)

// Keyring is the subset of OS-keychain operations the resolver and the auth
// command need, bound to spur's single token entry. The production
// implementation wraps zalando/go-keyring; tests inject an in-memory fake.
type Keyring interface {
	// Get returns the stored token, or "" (with a nil error) when none is set.
	Get() (string, error)
	// Set stores token, overwriting any existing one.
	Set(token string) error
	// Delete removes the stored token; removing an absent token is a no-op.
	Delete() error
}

// osKeyring is the production Keyring backed by the OS keychain (Keychain on
// macOS, Secret Service on Linux, Credential Manager on Windows).
type osKeyring struct{}

// DefaultKeyring returns the Keyring backed by the real OS keychain.
func DefaultKeyring() Keyring { return osKeyring{} }

func (osKeyring) Get() (string, error) {
	tok, err := keyring.Get(keychainService, keychainAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		// A missing entry is "no token", not a failure: the resolver falls
		// through to ErrNoToken and the caller prints the how-to-authenticate
		// hint rather than a keychain error.
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return tok, nil
}

func (osKeyring) Set(token string) error {
	return keyring.Set(keychainService, keychainAccount, token)
}

func (osKeyring) Delete() error {
	err := keyring.Delete(keychainService, keychainAccount)
	if errors.Is(err, keyring.ErrNotFound) {
		// Logging out with nothing stored is a success, not an error.
		return nil
	}
	return err
}

// Store saves token in the keychain via kr. It rejects an empty token so a
// blank prompt can never overwrite a real stored credential with nothing.
func Store(kr Keyring, token string) error {
	if token == "" {
		return errors.New("refusing to store an empty token")
	}
	if err := kr.Set(token); err != nil {
		return fmt.Errorf("store token in keychain: %w", err)
	}
	return nil
}

// Logout removes any stored token via kr. It is idempotent: clearing an already
// empty keychain succeeds.
func Logout(kr Keyring) error {
	if err := kr.Delete(); err != nil {
		return fmt.Errorf("remove token from keychain: %w", err)
	}
	return nil
}
