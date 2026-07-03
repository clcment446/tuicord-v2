package keyring

import "github.com/zalando/go-keyring"

const service = "tuicord"

func GetToken() (string, error) {
	return kr.Get(service, "token")
}

func SetToken(token string) error {
	return kr.Set(service, "token", token)
}

func DeleteToken() error {
	return kr.Delete(service, "token")
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
