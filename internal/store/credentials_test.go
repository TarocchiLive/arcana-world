package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"arcana-world/internal/domain"
	"github.com/zalando/go-keyring"
)

func TestFileFallbackSurvivesKeyringRecoveryAndDeletion(t *testing.T) {
	dir := t.TempDir()
	backend := &automaticBackend{dir: dir, system: memoryBackend{}, probe: func() (bool, error) { return true, nil }}
	s, err := OpenWithBackend(dir, backend)
	if err != nil {
		t.Fatal(err)
	}
	a := domain.Account{UID: "42", Cookies: map[string]string{"SESSDATA": "saved-cookie"}}
	if err := s.Save(a); err != nil {
		t.Fatal(err)
	}
	if s.CredentialStorage() != StorageFile {
		t.Fatal("successful fallback did not select file storage")
	}
	if err := s.SetOBSSecret("obs-secret"); err != nil {
		t.Fatal(err)
	}
	recovered := &automaticBackend{dir: dir, system: memoryBackend{}, probe: func() (bool, error) { return false, nil }}
	reopened, err := OpenWithBackend(dir, recovered)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := reopened.Load(a.UID)
	if err != nil || loaded.Cookies["SESSDATA"] != "saved-cookie" {
		t.Fatalf("recovery hid saved login: %v", err)
	}
	if password, err := reopened.OBSSecret(); err != nil || password != "obs-secret" {
		t.Fatalf("recovery hid OBS password: %v", err)
	}
	if err := reopened.Delete(a.UID); err != nil {
		t.Fatal(err)
	}
	if _, err := recovered.Get("account:42"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("deleted login remains: %v", err)
	}
	if err := reopened.ClearData(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "credentials", "secrets.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clear left plaintext credentials: %v", err)
	}
}

func TestAvailableOrLockedKeyringNeverCreatesPlaintext(t *testing.T) {
	for _, locked := range []bool{false, true} {
		t.Run(map[bool]string{false: "available", true: "locked"}[locked], func(t *testing.T) {
			dir := t.TempDir()
			system := &unavailableBackend{memoryBackend: memoryBackend{}, unavailable: locked}
			backend := &automaticBackend{dir: dir, system: system, probe: func() (bool, error) { return false, nil }}
			s, err := OpenWithBackend(dir, backend)
			if err != nil {
				t.Fatal(err)
			}
			err = s.Save(domain.Account{UID: "42", Cookies: map[string]string{"SESSDATA": "cookie"}})
			if (err != nil) != locked {
				t.Fatalf("unexpected save result: %v", err)
			}
			if s.CredentialStorage() == StorageFile {
				t.Fatal("keyring operation selected plaintext storage")
			}
			if _, err := os.Lstat(filepath.Join(dir, "credentials")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("keyring operation created plaintext directory: %v", err)
			}
			if !locked {
				loaded, err := s.Load("42")
				if err != nil || loaded.Cookies["SESSDATA"] != "cookie" {
					t.Fatalf("keyring save cannot be read: %v", err)
				}
			}
		})
	}
}

func TestKeyringProbeFailureDoesNotDowngrade(t *testing.T) {
	dir := t.TempDir()
	denied := errors.New("access denied")
	backend := &automaticBackend{dir: dir, system: memoryBackend{}, probe: func() (bool, error) { return false, denied }}
	if err := backend.Set("user", "secret"); !errors.Is(err, denied) {
		t.Fatalf("probe error lost: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "credentials")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("probe failure created plaintext: %v", err)
	}
}
