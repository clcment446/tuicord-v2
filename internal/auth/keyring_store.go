package auth

import "awesomeProject/internal/keyring"

type KeyringStore struct{}

func (KeyringStore) GetToken() (string, error) {
	return keyring.GetToken()
}

func (KeyringStore) SetToken(token string) error {
	return keyring.SetToken(token)
}

func (KeyringStore) DeleteToken() error {
	return keyring.DeleteToken()
}
