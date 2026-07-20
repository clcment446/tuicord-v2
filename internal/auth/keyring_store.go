package auth

import "awesomeProject/internal/keyring"

// KeyringStore reads and writes a single account's token in the OS keyring.
// A zero-value KeyringStore{} targets the legacy single-account token key, so
// existing callers keep working; set Key to scope the store to one account in
// the multi-account registry.
type KeyringStore struct {
	// Key is the per-account keyring key. Empty means the legacy "token" key.
	Key string
}

func (s KeyringStore) key() string {
	if s.Key == "" {
		return keyring.LegacyTokenKey
	}
	return s.Key
}

func (s KeyringStore) GetToken() (string, error) {
	return keyring.GetTokenFor(s.key())
}

func (s KeyringStore) SetToken(token string) error {
	return keyring.SetTokenFor(s.key(), token)
}

func (s KeyringStore) DeleteToken() error {
	return keyring.DeleteTokenFor(s.key())
}
