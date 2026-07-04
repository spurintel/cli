// Package auth resolves the active Spur API token and manages its storage in
// the OS keychain.
//
// Token resolution is a deep module: callers hand it the environment and
// config-file values and a Keyring, and it returns the active token using a
// fixed precedence — the SPUR_TOKEN environment variable, then the config file,
// then the OS keychain. A higher tier short-circuits the lower ones, so a token
// in the environment never triggers a keychain read.
package auth

import (
	"errors"
	"fmt"
)

// EnvVar is the environment variable that carries the token at the highest
// precedence. It is the long-standing SPUR_TOKEN convention the whole tool
// (and the spur-mcp server before it) already honors.
const EnvVar = "SPUR_TOKEN"

// ErrNoToken is returned by Resolve when no source supplies a token. Callers
// surface it as an actionable message pointing at the ways to provide one.
var ErrNoToken = errors.New("no Spur API token found")

// Resolve returns the active token using the precedence env → config → keychain.
//
// env and cfg are the already-read values from the environment and the config
// file; passing them in (rather than reading them here) keeps the precedence
// logic pure and lets tests drive every combination. kr is consulted only when
// both higher tiers are empty, so a hit above never touches the keychain.
func Resolve(env, cfg string, kr Keyring) (string, error) {
	if env != "" {
		return env, nil
	}
	if cfg != "" {
		return cfg, nil
	}
	tok, err := kr.Get()
	if err != nil {
		return "", fmt.Errorf("read token from keychain: %w", err)
	}
	if tok != "" {
		return tok, nil
	}
	return "", ErrNoToken
}
