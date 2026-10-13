package store

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"arcana-world/internal/domain"
	"arcana-world/internal/journal"
)

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

func TestSaveConfigPreservesAccountIndexAndOwnsHistory(t *testing.T) {
	s, err := OpenWithBackend(t.TempDir(), memoryBackend{})
	if err != nil {
		t.Fatal(err)
	}
	account := domain.Account{UID: "saved", Name: "original", Cookies: map[string]string{"SESSDATA": "secret"}}
	if err := s.Save(account); err != nil {
		t.Fatal(err)
	}
	cfg := s.Config()
	cfg.Accounts = []domain.AccountInfo{{UID: "injected", Name: "not saved"}}
	cfg.RecentTitles = []string{"saved title"}
	cfg.RecentAreas = []domain.Area{{ID: 7, Name: "saved area"}}
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.RecentTitles[0] = "mutated"
	cfg.RecentAreas[0].Name = "mutated"
	got := s.Config()
	if len(got.Accounts) != 1 || got.Accounts[0].UID != account.UID {
		t.Fatal("settings save replaced the credential-backed account index")
	}
	if got.RecentTitles[0] != "saved title" || got.RecentAreas[0].Name != "saved area" {
		t.Fatal("settings save retained caller-owned history slices")
	}
	if loaded, err := s.Load(account.UID); err != nil || loaded.Name != account.Name {
		t.Fatalf("settings save made the saved account unavailable: %v", err)
	}
	path := filepath.Join(s.Dir(), "config.json")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(before, &fields); err != nil {
		t.Fatal(err)
	}
	fields["future_setting"] = json.RawMessage(`{"precise":9007199254740993,"nested":{"enabled":true}}`)
	before, err = json.MarshalIndent(fields, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, before, 0600); err != nil {
		t.Fatal(err)
	}
	lock, err := journal.LockDirectory(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lock.Close() })
	if err := SaveDefaultUI(s.Dir(), "webui"); err == nil {
		t.Fatal("startup preference save ignored the active instance lock")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("blocked preference save changed configuration: %v", err)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	if err := SaveDefaultUI(s.Dir(), "webui"); err != nil {
		t.Fatal(err)
	}
	saved, err := ReadConfig(s.Dir())
	if err != nil {
		t.Fatal(err)
	}
	got.DefaultUI = "webui"
	if !reflect.DeepEqual(saved, got) {
		t.Fatal("startup preference save changed account index or other settings")
	}
	after, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var savedFields map[string]json.RawMessage
	if err := json.Unmarshal(after, &savedFields); err != nil {
		t.Fatal(err)
	}
	delete(savedFields, "default_ui")
	delete(fields, "default_ui")
	expected, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := json.Marshal(savedFields)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatal("startup preference save changed unrelated raw fields")
	}
}
