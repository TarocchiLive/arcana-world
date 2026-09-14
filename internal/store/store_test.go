package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"arcana-world/internal/domain"
	"github.com/zalando/go-keyring"
)

type memoryBackend map[string]string

func (b memoryBackend) Get(service, user string) (string, error) {
	v, ok := b[service+user]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return v, nil
}
func (b memoryBackend) Set(service, user, password string) error {
	b[service+user] = password
	return nil
}
func (b memoryBackend) Delete(service, user string) error { delete(b, service+user); return nil }

func TestFailedIndexCommitRestoresPreviousCredential(t *testing.T) {
	dir := t.TempDir()
	backend := memoryBackend{}
	s, err := OpenWithBackend(dir, backend)
	if err != nil {
		t.Fatal(err)
	}
	original := domain.Account{UID: "1", Name: "first", Cookies: map[string]string{"DedeUserID": "1", "SESSDATA": "old-secret", "bili_jct": "csrf"}}
	if err = s.Save(original); err != nil {
		t.Fatal(err)
	}
	// 在密钥环写入成功后，强制让实际的原子重命名失败。
	index := filepath.Join(dir, "config.json")
	backup := filepath.Join(dir, "saved-index")
	if err = os.Rename(index, backup); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(index, 0700); err != nil {
		t.Fatal(err)
	}
	replacement := domain.Account{UID: "1", Name: "replacement", Cookies: map[string]string{"DedeUserID": "1", "SESSDATA": "new-secret", "bili_jct": "csrf"}}
	if err = s.Save(replacement); err == nil {
		t.Fatal("replacement succeeded without committed index")
	}
	loaded, err := s.Load("1")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Cookies["SESSDATA"] != "old-secret" || loaded.Name != "first" {
		t.Fatal("failed transaction published replacement account")
	}
	if err = s.Delete("1"); err == nil {
		t.Fatal("deletion succeeded without committed index")
	}
	loaded, err = s.Load("1")
	if err != nil || loaded.Cookies["SESSDATA"] != "old-secret" {
		t.Fatal("failed deletion lost the credential")
	}
	if err = os.Remove(index); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(backup, index); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenWithBackend(dir, backend)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = reopened.Load("1")
	if err != nil || loaded.Name != "first" || loaded.Cookies["SESSDATA"] != "old-secret" {
		t.Fatal("rollback did not survive reopening")
	}
}

type unavailableBackend struct {
	memoryBackend
	unavailable bool
}

func (b *unavailableBackend) Get(s, u string) (string, error) {
	if b.unavailable {
		return "", errors.New("service locked")
	}
	return b.memoryBackend.Get(s, u)
}
func TestTransientKeyringFailurePreservesAccount(t *testing.T) {
	backend := &unavailableBackend{memoryBackend: memoryBackend{}}
	s, err := OpenWithBackend(t.TempDir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	a := domain.Account{UID: "1", Name: "first", Cookies: map[string]string{"DedeUserID": "1", "SESSDATA": "secret", "bili_jct": "csrf"}}
	if err = s.Save(a); err != nil {
		t.Fatal(err)
	}
	backend.unavailable = true
	if err = s.Delete("1"); err == nil {
		t.Fatal("deletion accepted unavailable keyring")
	}
	backend.unavailable = false
	loaded, err := s.Load("1")
	if err != nil || loaded.Cookies["SESSDATA"] != "secret" {
		t.Fatal("temporary failure destroyed account")
	}
}

func TestDefaultDataDirectoryUsesHomeNotXDG(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := OpenWithBackend("", memoryBackend{})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(home, ".arcana", "world")
	if s.Dir() != want {
		t.Fatalf("data directory = %q, want %q", s.Dir(), want)
	}
	if _, err := os.Stat(filepath.Join(want, "config.json")); err != nil {
		t.Fatal(err)
	}
}
