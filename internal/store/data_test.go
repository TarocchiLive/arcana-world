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
	c.TUITheme = "midnight"
	c.TUIMotionDisabled = true
	c.TUICompactHeader = true
	c.TUINotificationsWarningsOnly = true
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
	want.TUITheme = d.TUITheme
	want.TUIMotionDisabled = d.TUIMotionDisabled
	want.TUICompactHeader = d.TUICompactHeader
	want.TUINotificationsWarningsOnly = d.TUINotificationsWarningsOnly
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
	if reopened.DeviceID() != s.DeviceID() {
		t.Fatal("reset or reopen changed the device identity")
	}
	loaded, err := reopened.Load(a.UID)
	if err != nil || !reflect.DeepEqual(loaded, a) {
		t.Fatalf("account changed: %v", err)
	}
	if secret, err := reopened.OBSSecret(); err != nil || secret != "obs secret" {
		t.Fatalf("OBS secret changed: %v", err)
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
	other, err := OpenWithBackend(t.TempDir(), memoryBackend{})
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
	owned = append(owned, deviceFile)
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
		if _, err := backend.Get(user); !errors.Is(err, keyring.ErrNotFound) {
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
	reopened, err := OpenWithBackend(s.Dir(), backend)
	if err != nil {
		t.Fatal(err)
	}
	if reopened.DeviceID() == s.DeviceID() {
		t.Fatal("cleared device identity was reused")
	}
}
