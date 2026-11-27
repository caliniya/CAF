package DesktopBackend

import (
	"encoding/base64"
	"errors"

	"github.com/zalando/go-keyring"
)

// Each data directory keeps its auto-login record and model-sharing key in separate native entries.
type deviceKeys struct{}

const vaultService = "CAF Vault"

func (deviceKeys) Load(id string) ([]byte, error) {
	v, err := keyring.Get(vaultService, id)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(v)
}
func (deviceKeys) Save(id string, key []byte) error {
	return keyring.Set(vaultService, id, base64.StdEncoding.EncodeToString(key))
}
func (deviceKeys) Forget(id string) error {
	err := keyring.Delete(vaultService, id)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
