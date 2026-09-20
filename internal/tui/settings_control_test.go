package tui

import (
	"arcana-world/internal/bili"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"arcana-world/internal/domain"
	"arcana-world/internal/store"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/zalando/go-keyring"
)

func TestResetSettingsRequiresConfirmationAndKeepsOBS(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	state := &lifecycleOBS{}
	state.active.Store(true)
	endpoint := lifecycleServer(t, state)
	cfg := m.store.Config()
	cfg.Proxy, cfg.Protocol = "direct", "srt"
	cfg.OBSURL, cfg.OBSAutoConnect = endpoint, true
	cfg.ExitOBSStopDisabled, cfg.ExitLiveStopDisabled = true, true
	cfg.Overlay.Enabled = true
	cfg.Overlay.Font.Size = 32
	if err := m.store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	account := domain.Account{UID: "42", Cookies: map[string]string{"SESSDATA": "saved-cookie"}}
	if err := m.store.Save(account); err != nil {
		t.Fatal(err)
	}
	if err := m.store.SetOBSSecret("saved-password"); err != nil {
		t.Fatal(err)
	}
	if err := m.obsClient.Connect(context.Background(), endpoint, ""); err != nil {
		t.Fatal(err)
	}
	m.config, m.overlayEnabled, m.page = m.store.Config(), true, settingsPage
	before := m.store.Config()
	m.perform("settings-reset-confirm")
	if cmd := m.modalKey(tea.KeyMsg{Type: tea.KeyEnter}); cmd != nil {
		t.Fatal("default confirmation choice did not cancel")
	}
	if !reflect.DeepEqual(m.store.Config(), before) {
		t.Fatal("canceling reset changed saved settings")
	}
	m.perform("settings-reset-confirm")
	m.modalKey(tea.KeyMsg{Type: tea.KeyRight})
	cmd := m.modalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirmed reset did not run")
	}
	msg := cmd().(taskMessage)
	if msg.taskError() != nil {
		t.Fatal(msg.taskError())
	}
	m.Update(msg)
	got, defaults := m.store.Config(), store.DefaultConfig()
	if got.Proxy != defaults.Proxy || got.Protocol != defaults.Protocol || !reflect.DeepEqual(got.Overlay, defaults.Overlay) || got.ExitOBSStopDisabled || got.ExitLiveStopDisabled || m.overlayEnabled {
		t.Fatal("settings reset did not apply to the persisted and running model")
	}
	if got.OBSURL != endpoint || !got.OBSAutoConnect || !m.obsClient.Snapshot().Connected || !state.active.Load() || state.stops.Load() != 0 {
		t.Fatal("settings reset disturbed OBS configuration or streaming")
	}
	if saved, err := m.store.Load(account.UID); err != nil || !reflect.DeepEqual(saved, account) {
		t.Fatalf("reset lost Bilibili credentials: %v", err)
	}
	if password, err := m.store.OBSSecret(); err != nil || password != "saved-password" {
		t.Fatalf("reset lost OBS credentials: %v", err)
	}
}

func TestResetSettingsClosesPreviousIdleConnections(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	closed := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	server.Start()
	defer server.Close()
	oldHTTP := m.client.HTTP
	defer oldHTTP.CloseIdleConnections()
	response, err := oldHTTP.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	msg := m.resetSettings()().(taskMessage)
	if msg.taskError() != nil {
		t.Fatal(msg.taskError())
	}
	m.Update(msg)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("reset left the previous client's idle connection open")
	}
}

func TestClearDataRequiresExactPhraseAndClosesBeforeDeletion(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.page = settingsPage
	if err := m.store.SetOBSSecret("saved-password"); err != nil {
		t.Fatal(err)
	}
	if err := m.chat.history.RecordGap(42, "retained-history"); err != nil {
		t.Fatal(err)
	}
	m.log("retained-log")
	configPath := filepath.Join(m.store.Dir(), "config.json")
	m.perform("clear-data")
	m.input.SetValue("arcanaworldclear!")
	previousStatus := m.status
	if cmd := m.submitForm(); cmd != nil {
		t.Fatal("incorrect confirmation initiated clearing")
	}
	if m.mode != "" || m.page != settingsPage || m.status == previousStatus || m.input.Value() != "" {
		t.Fatal("incorrect confirmation did not return to settings with a message")
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatal("incorrect confirmation removed data")
	}
	if password, err := m.store.OBSSecret(); err != nil || password != "saved-password" {
		t.Fatal("incorrect confirmation removed credentials")
	}
	m.perform("clear-data")
	m.input.SetValue("arcanaworldclear")
	cmd := m.submitForm()
	if cmd == nil {
		t.Fatal("exact confirmation did not initiate exit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("clearing must exit before deleting open data files")
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatal("data was deleted before shutdown")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"config.json", "danmaku/history.db", "logs/arcana-world.log", "logs/journal.lock"} {
		if _, err := os.Stat(filepath.Join(m.store.Dir(), name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("owned data remains after clearing: %s: %v", name, err)
		}
	}
	if _, err := m.store.OBSSecret(); !errors.Is(err, keyring.ErrNotFound) {
		t.Fatalf("OBS password remains after clearing: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	m.Update(taskResult[*bili.Client]{operation: configOperation()})
	if _, err := os.Stat(configPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("late update or repeated close recreated cleared data")
	}
}

func TestClearDataDoesNotEraseCredentialsWhenShutdownFails(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	state := &lifecycleOBS{rejectStop: true}
	state.active.Store(true)
	endpoint := lifecycleServer(t, state)
	if err := m.obsClient.Connect(context.Background(), endpoint, ""); err != nil {
		t.Fatal(err)
	}
	if err := m.store.SetOBSSecret("saved-password"); err != nil {
		t.Fatal(err)
	}
	m.perform("clear-data")
	m.input.SetValue("arcanaworldclear")
	m.submitForm()
	if err := m.Close(); err == nil {
		t.Fatal("clear hid shutdown failure")
	}
	if password, err := m.store.OBSSecret(); err != nil || password != "saved-password" {
		t.Fatal("failed shutdown erased OBS credentials")
	}
	if _, err := os.Stat(filepath.Join(m.store.Dir(), "config.json")); err != nil {
		t.Fatal("failed shutdown erased configuration")
	}
}

func TestRestoreOverlayAppearanceRequiresConfirmationAndKeepsContent(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	cfg := m.store.Config()
	cfg.Overlay.Content = "combined"
	cfg.Overlay.Font.Size = 32
	cfg.Overlay.Position.X = 80
	cfg.Proxy = "direct"
	if err := m.store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	m.config = cfg
	m.overlayEnabled = true // A session-only override must survive appearance changes.
	m.perform("overlay-settings")
	m.selected = len(m.choices) - 1
	if cmd := m.choose(); cmd != nil || m.mode != "confirm" {
		t.Fatal("restore did not wait for confirmation")
	}
	m.modalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !reflect.DeepEqual(m.store.Config(), cfg) {
		t.Fatal("canceling restore changed saved settings")
	}
	m.perform("overlay-settings")
	m.selected = len(m.choices) - 1
	m.choose()
	m.modalKey(tea.KeyMsg{Type: tea.KeyRight})
	cmd := m.modalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirmed appearance restore did not save")
	}
	msg := cmd().(taskMessage)
	if msg.taskError() != nil {
		t.Fatal(msg.taskError())
	}
	m.Update(msg)
	want := cfg
	want.Overlay = store.DefaultConfig().Overlay
	want.Overlay.Enabled, want.Overlay.Content = cfg.Overlay.Enabled, cfg.Overlay.Content
	if !reflect.DeepEqual(m.store.Config(), want) || !m.overlayEnabled {
		t.Fatal("appearance restore changed content, enable state or unrelated settings")
	}
}
