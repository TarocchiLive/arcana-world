package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestCredentialFileRejectsSymlinksAndSecuresExistingFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions and unprivileged symlinks")
	}
	dir := t.TempDir()
	backend := newFileBackend(dir)
	if _, err := backend.Get("account"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "credentials")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("reading absent credentials created a directory")
	}
	if err := backend.Set("account", "old-secret"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "credentials", "secrets.json")
	if err := os.Chmod(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if value, err := backend.Get("account"); err != nil || value != "old-secret" {
		t.Fatalf("read failed: %v", err)
	}
	for _, check := range []struct {
		path string
		mode os.FileMode
	}{{filepath.Dir(path), 0700}, {path, 0600}} {
		info, err := os.Stat(check.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != check.mode {
			t.Fatalf("unsafe permissions: %o", info.Mode().Perm())
		}
	}
	target := filepath.Join(dir, "outside.json")
	if err := os.Rename(path, target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if err := backend.Set("account", "replacement"); err == nil {
		t.Fatal("write followed a credential symlink")
	}
	if _, err := backend.Get("account"); err == nil {
		t.Fatal("read followed a credential symlink")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	var values map[string]string
	if err := json.Unmarshal(data, &values); err != nil {
		t.Fatal(err)
	}
	if values["account"] != "old-secret" {
		t.Fatal("rejected write modified symlink target")
	}
}
