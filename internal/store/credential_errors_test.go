package store

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestSecretErrorSanitizesCauses(t *testing.T) {
	const secret = "private-credential-value"
	cases := []struct {
		name     string
		cause    error
		category error
	}{
		{"unavailable", errors.New(secret), ErrCredentialUnavailable},
		{"permission", &os.PathError{Op: "open", Path: secret, Err: os.ErrPermission}, ErrCredentialPermission},
		{"corrupt", fmt.Errorf("%s: %w", secret, ErrCredentialCorrupt), ErrCredentialCorrupt},
		{"not found", fmt.Errorf("%s: %w", secret, keyring.ErrNotFound), keyring.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := secretError("save credentials", tc.cause)
			if !errors.Is(err, tc.category) {
				t.Fatalf("error does not match category %v", tc.category)
			}
			if errors.Is(err, tc.cause) {
				t.Fatal("external cause remains reachable")
			}
			for current := err; current != nil; current = errors.Unwrap(current) {
				if strings.Contains(current.Error(), secret) {
					t.Fatal("credential leaked through error chain")
				}
			}
		})
	}
}
