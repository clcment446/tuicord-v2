package keyring

import "github.com/zalando/go-keyring"

const service = "tuicord"

// ErrNotFound is returned by Get/Delete when no token is stored under the key.
// It distinguishes the normal first-run/logged-out state from a real keyring
// failure (locked daemon, missing Secret Service, ...).
var ErrNotFound = keyring.ErrNotFound

// LegacyTokenKey is the go-keyring "user" under which the original
// single-account token is stored. It doubles as the keyring key of the first
// account when the single-account setup is migrated into the registry.
const LegacyTokenKey = "token"

func GetToken() (string, error) {
	return GetTokenFor(LegacyTokenKey)
}

func SetToken(token string) error {
	return SetTokenFor(LegacyTokenKey, token)
}

func DeleteToken() error {
	return DeleteTokenFor(LegacyTokenKey)
}

// GetTokenFor reads the token stored under the given per-account key.
func GetTokenFor(key string) (string, error) {
	return kr.Get(service, key)
}

// SetTokenFor stores an account's token under the given per-account key.
func SetTokenFor(key, token string) error {
	return kr.Set(service, key, token)
}

// DeleteTokenFor removes the token stored under the given per-account key.
func DeleteTokenFor(key string) error {
	return kr.Delete(service, key)
}

var kr keyringBackend = realKeyring{}

type keyringBackend interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}

type realKeyring struct{}

func (realKeyring) Get(s, u string) (string, error) { return keyring.Get(s, u) }
func (realKeyring) Set(s, u, p string) error        { return keyring.Set(s, u, p) }
func (realKeyring) Delete(s, u string) error        { return keyring.Delete(s, u) }
