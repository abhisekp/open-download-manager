package credentials

import (
	"errors"
	"github.com/zalando/go-keyring"
)

type OSBackend struct{}

func (OSBackend) Set(service, id, value string) error    { return keyring.Set(service, id, value) }
func (OSBackend) Get(service, id string) (string, error) { return keyring.Get(service, id) }
func (OSBackend) Delete(service, id string) error        { return keyring.Delete(service, id) }
func (OSBackend) Missing(err error) bool                 { return errors.Is(err, keyring.ErrNotFound) }
