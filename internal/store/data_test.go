package store

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"arcana-world/internal/domain"
	"github.com/zalando/go-keyring"
)

func TestResetSettingsPreservesConnectionsAccountsAndHistory(t *testing.T) {
	backend := memoryBackend{}
	s, err := OpenWithBackend(t.TempDir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	a := domain.Account{UID: "42", Name: "saved", Cookies: map[string]string{"SESSDATA": "secret"}}
	if err := s.Save(a); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOBSSecret("obs secret"); err != nil {
		t.Fatal(err)
	}
	c := s.Config()
	c.ActiveUID = a.UID
	c.OBSURL = "ws://localhost:1234"
	c.OBSAutoConnect = true
	c.OBSAutoStream = true
	c.DanmakuDisabled = true
	c.RecentTitles = []string{"saved title"}
	c.RecentAreas = []domain.Area{{ID: 12, Name: "saved area"}}
	c.Proxy = "direct"
	c.Protocol = "srt"
	c.ExitOBSStopDisabled = true
	c.ExitLiveStopDisabled = true
	c.Overlay.Enabled = true
	c.Overlay.Content = "combined"
	if err := s.SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	want := c
	d := DefaultConfig()
	want.Proxy, want.Protocol = d.Proxy, d.Protocol
	want.ExitOBSStopDisabled, want.ExitLiveStopDisabled = d.ExitOBSStopDisabled, d.ExitLiveStopDisabled
	want.Overlay = d.Overlay
	got, err := s.ResetSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reset changed unrelated preferences: got %+v, want %+v", got, want)
	}
	reopened, err := OpenWithBackend(s.Dir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(reopened.Config(), want) {
		t.Fatal("reset did not persist")
	}
	loaded, err := reopened.Load(a.UID)
	if err != nil || !reflect.DeepEqual(loaded, a) {
		t.Fatalf("account changed: %v", err)
	}
	if secret, err := reopened.OBSSecret(); err != nil || secret != "obs secret" {
		t.Fatalf("OBS secret changed: %v", err)
	}
}

func TestClearDataRemovesUnlistedActiveAccountCredentials(t *testing.T) {
	backend := memoryBackend{}
	s, err := OpenWithBackend(t.TempDir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Save(domain.Account{UID: "42", Cookies: map[string]string{"SESSDATA": "secret"}}); err != nil {
		t.Fatal(err)
	}
	// A stale or manually edited legacy index can retain only the active UID.
	if err := os.WriteFile(filepath.Join(s.Dir(), "config.json"), []byte(`{"active_uid":"42"}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithBackend(s.Dir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ClearData(); err != nil {
		t.Fatal(err)
	}
	if _, err := backend.Get(s.service, "account:42"); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("clearing left known active-account credentials behind: %v", err)
	}
}

func TestClearDataRemovesOwnedFilesAndRejectsStaleWrites(t *testing.T) {
	backend := memoryBackend{}
	s, err := OpenWithBackend(t.TempDir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	a := domain.Account{UID: "42", Cookies: map[string]string{"SESSDATA": "secret"}}
	if err := s.Save(a); err != nil {
		t.Fatal(err)
	}
	if err := s.SetOBSSecret("obs secret"); err != nil {
		t.Fatal(err)
	}
	other, err := OpenWithBackend(t.TempDir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	if err := other.Save(a); err != nil {
		t.Fatal(err)
	}
	owned := []string{"config.json", ".config-abandoned", "danmaku/history.db", "logs/arcana-world.log", "logs/journal.lock", "logs/.journal-abandoned"}
	unrelated := []string{"personal.txt", "logs/personal.log", "danmaku/personal.db", ".config-directory/keep"}
	for _, name := range append(append([]string{}, owned[1:]...), unrelated...) {
		path := filepath.Join(s.Dir(), name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	stale := s.Config()
	if err := s.ClearData(); err != nil {
		t.Fatal(err)
	}
	for _, name := range owned {
		if _, err := os.Lstat(filepath.Join(s.Dir(), name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned file %s remains: %v", name, err)
		}
	}
	for _, name := range unrelated {
		if data, err := os.ReadFile(filepath.Join(s.Dir(), name)); err != nil || string(data) != "keep" {
			t.Fatalf("unrelated file %s changed: %v", name, err)
		}
	}
	for _, user := range []string{"account:42", "obs-password"} {
		if _, err := backend.Get(s.service, user); !errors.Is(err, keyring.ErrNotFound) {
			t.Fatalf("credential %s remains", user)
		}
	}
	if _, err := other.Load(a.UID); err != nil {
		t.Fatalf("other profile credential removed: %v", err)
	}
	for name, write := range map[string]func() error{
		"config":  func() error { return s.SaveConfig(stale) },
		"account": func() error { return s.Save(a) },
		"delete":  func() error { return s.Delete(a.UID) },
		"OBS":     func() error { return s.SetOBSSecret("stale") },
		"reset":   func() error { _, err := s.ResetSettings(); return err },
		"persist": func() error { return s.persist(stale) },
	} {
		if err := write(); err == nil {
			t.Fatalf("stale %s write accepted", name)
		}
	}
	if err := s.ClearData(); err != nil {
		t.Fatalf("repeat clear failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "config.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("stale operation recreated config")
	}
}

type deleteFailureBackend struct {
	memoryBackend
	failUser string
}

func (b *deleteFailureBackend) Delete(service, user string) error {
	if user == b.failUser {
		return errors.New("keyring locked")
	}
	if _, err := b.Get(service, user); err != nil {
		return err
	}
	return b.memoryBackend.Delete(service, user)
}

func TestClearDataRetainsRetryIndexOnCredentialFailure(t *testing.T) {
	for _, failing := range []string{"account:2", "obs-password"} {
		t.Run(failing, func(t *testing.T) {
			backend := &deleteFailureBackend{memoryBackend: memoryBackend{}}
			s, err := OpenWithBackend(t.TempDir(), backend)
			if err != nil {
				t.Fatal(err)
			}
			for _, uid := range []string{"1", "2"} {
				if err := s.Save(domain.Account{UID: uid, Cookies: map[string]string{"SESSDATA": "secret"}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := s.SetOBSSecret("password"); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(filepath.Join(s.Dir(), "config.json"))
			if err != nil {
				t.Fatal(err)
			}
			backend.failUser = failing
			if err := s.ClearData(); err == nil {
				t.Fatal("credential failure hidden")
			}
			after, err := os.ReadFile(filepath.Join(s.Dir(), "config.json"))
			if err != nil || string(after) != string(before) {
				t.Fatal("retry index lost")
			}
			if err := s.SaveConfig(s.Config()); err == nil {
				t.Fatal("partial clear allowed stale write")
			}
			reopened, err := OpenWithBackend(s.Dir(), backend)
			if err != nil {
				t.Fatal(err)
			}
			backend.failUser = ""
			if err := reopened.ClearData(); err != nil {
				t.Fatalf("retry failed: %v", err)
			}
			if len(backend.memoryBackend) != 0 {
				t.Fatal("retry left secrets behind")
			}
		})
	}
}

func TestClearDataDoesNotFollowSymlinkTargets(t *testing.T) {
	for _, name := range []string{"config.json", "danmaku", "logs", "danmaku/history.db", "logs/arcana-world.log"} {
		t.Run(name, func(t *testing.T) {
			s, err := OpenWithBackend(t.TempDir(), memoryBackend{})
			if err != nil {
				t.Fatal(err)
			}
			outside := t.TempDir()
			for _, file := range []string{"history.db", "arcana-world.log", "journal.lock", ".journal-temp", "victim"} {
				if err := os.WriteFile(filepath.Join(outside, file), []byte("untouched"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(s.Dir(), name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if name == "config.json" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			target := outside
			if name != "danmaku" && name != "logs" {
				target = filepath.Join(outside, "victim")
			}
			if err := os.Symlink(target, path); err != nil {
				t.Fatal(err)
			}
			if err := s.ClearData(); err != nil {
				t.Fatal(err)
			}
			for _, file := range []string{"history.db", "arcana-world.log", "journal.lock", ".journal-temp", "victim"} {
				if data, err := os.ReadFile(filepath.Join(outside, file)); err != nil || string(data) != "untouched" {
					t.Fatalf("target %s changed: %v", file, err)
				}
			}
			if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("link remains: %v", err)
			}
		})
	}
}

func TestClearDataRetainsIndexAfterFileFailure(t *testing.T) {
	s, err := OpenWithBackend(t.TempDir(), memoryBackend{})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Dir(), "danmaku", "history.db")
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	// A directory is not a history database and must not be recursively removed.
	victim := filepath.Join(path, "unrelated")
	if err := os.WriteFile(victim, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearData(); err == nil {
		t.Fatal("file removal failure hidden")
	}
	if _, err := os.Stat(filepath.Join(s.Dir(), "config.json")); err != nil {
		t.Fatalf("retry config lost: %v", err)
	}
	if data, err := os.ReadFile(victim); err != nil || string(data) != "keep" {
		t.Fatal("unexpected directory contents removed")
	}
	if err := os.Remove(victim); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearData(); err != nil {
		t.Fatalf("file removal retry failed: %v", err)
	}
}
