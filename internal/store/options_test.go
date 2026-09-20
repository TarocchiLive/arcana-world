package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"arcana-world/internal/domain"
	"github.com/zalando/go-keyring"
)

func TestRuntimeOverridesNeverPersist(t *testing.T) {
	dir := t.TempDir()
	saved, err := Open(dir, Options{CredentialBackend: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	c := saved.Config()
	c.Proxy = "http://saved.example:8080"
	if err := saved.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	proxy, enabled := "", false
	s, err := Open(dir, Options{CredentialBackend: "memory", Overrides: ConfigOverrides{Proxy: &proxy, OBSAutoConnect: &enabled, OBSAutoStream: &enabled}})
	if err != nil {
		t.Fatal(err)
	}
	proxy, enabled = "http://mutated.example", true
	detached := s.Overrides()
	*detached.Proxy, *detached.OBSAutoConnect = "http://other.example", true
	assertEffective := func(c domain.Config) {
		t.Helper()
		if c.Proxy != "" || c.OBSAutoConnect || c.OBSAutoStream {
			t.Fatalf("session overrides changed: %+v", c)
		}
	}
	assertSaved := func(proxy string) {
		t.Helper()
		reopened, err := Open(dir, Options{CredentialBackend: "memory"})
		if err != nil {
			t.Fatal(err)
		}
		c := reopened.Config()
		if c.Proxy != proxy || !c.OBSAutoConnect || !c.OBSAutoStream {
			t.Fatalf("runtime settings leaked to disk: %+v", c)
		}
	}
	assertEffective(s.Config())
	c = s.Config()
	c.RecentTitles = []string{"unrelated"}
	if err := s.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	assertSaved("http://saved.example:8080")
	account := domain.Account{UID: "1", Cookies: map[string]string{"SESSDATA": "secret"}}
	if err := s.Save(account); err != nil {
		t.Fatal(err)
	}
	assertSaved("http://saved.example:8080")
	if err := s.Delete(account.UID); err != nil {
		t.Fatal(err)
	}
	assertSaved("http://saved.example:8080")
	reset, err := s.ResetSettings()
	if err != nil {
		t.Fatal(err)
	}
	assertEffective(reset)
	assertEffective(s.Config())
	assertSaved(DefaultConfig().Proxy)
}

func TestMemoryCredentialsAreIsolatedAcrossOpens(t *testing.T) {
	dir := t.TempDir()
	file, err := Open(dir, Options{CredentialBackend: "file"})
	if err != nil {
		t.Fatal(err)
	}
	if err := file.SetOBSSecret("persisted-secret"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials", credentialFile)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir, Options{CredentialBackend: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	if secret, err := s.OBSSecret(); !errors.Is(err, keyring.ErrNotFound) || secret != "" {
		t.Fatalf("memory read file credential: %q, %v", secret, err)
	}
	if err := s.SetOBSSecret("session-secret"); err != nil {
		t.Fatal(err)
	}
	if err := s.Save(domain.Account{UID: "1", Cookies: map[string]string{"SESSDATA": "session-cookie"}}); err != nil {
		t.Fatal(err)
	}
	if secret, err := s.OBSSecret(); err != nil || secret != "session-secret" {
		t.Fatalf("memory lost session credential: %q, %v", secret, err)
	}
	reopened, err := Open(dir, Options{CredentialBackend: "memory"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reopened.Load("1"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("memory account credential survived reopen: %v", err)
	}
	if secret, err := reopened.OBSSecret(); !errors.Is(err, keyring.ErrNotFound) || secret != "" {
		t.Fatalf("memory OBS credential survived reopen: %q, %v", secret, err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("memory backend changed persisted credentials")
	}
}
