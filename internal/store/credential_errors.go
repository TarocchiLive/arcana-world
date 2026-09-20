package store

import (
	"errors"
	"fmt"
	"os"

	"arcana-world/internal/i18n"
	"github.com/zalando/go-keyring"
)

var (
	ErrCredentialUnavailable = errors.New("credential store unavailable")
	ErrCredentialPermission  = errors.New("credential access denied")
	ErrCredentialCorrupt     = errors.New("credential data corrupt")
)

type credentialError struct {
	message  string
	category error
}

func (e *credentialError) Error() string { return e.message }
func (e *credentialError) Unwrap() error { return e.category }

func secretError(action string, err error) error {
	if errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf(i18n.T(i18n.StoreCredentialNotFound), action, keyring.ErrNotFound)
	}
	category := ErrCredentialUnavailable
	switch {
	case errors.Is(err, os.ErrPermission), errors.Is(err, ErrCredentialPermission):
		category = ErrCredentialPermission
	case errors.Is(err, ErrCredentialCorrupt):
		category = ErrCredentialCorrupt
	}
	return &credentialError{
		message:  fmt.Sprintf(i18n.T(i18n.StoreCredentialStoreUnavailable), action),
		category: category,
	}
}
