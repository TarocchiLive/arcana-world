package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"arcana-world/internal/domain"
	"arcana-world/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

type switchSecrets struct {
	lifecycleSecrets
	reject bool
}

func (s *switchSecrets) Set(service, user, value string) error {
	if s.reject && user == "account:7" {
		return fmt.Errorf("credential store rejected target")
	}
	return s.lifecycleSecrets.Set(service, user, value)
}

func switchModel(t *testing.T, failure string) (*Model, domain.Account, *atomic.Int32, *switchSecrets) {
	t.Helper()
	secrets := &switchSecrets{lifecycleSecrets: lifecycleSecrets{}}
	storage, err := store.OpenWithBackend(t.TempDir(), secrets)
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), storage)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	old := exitAccount()
	persistExitAccount(t, m, old)
	m.account = &old
	m.client.SetAccount(old)
	cfg := m.store.Config()
	cfg.ExitOBSStopDisabled = true
	cfg.ExitLiveStopDisabled = true
	if err := m.store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	m.config = cfg
	target := domain.Account{UID: "7", Name: "target", Cookies: map[string]string{"DedeUserID": "7", "SESSDATA": "new", "bili_jct": "new-csrf"}}
	if err := m.store.Save(target); err != nil {
		t.Fatal(err)
	}
	var requests, stops atomic.Int32
	m.client.LiveBase = exitLiveServer(t, true, failure, &requests, &stops)
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/x/web-interface/nav" {
			t.Errorf("unexpected request %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"code":0,"data":{"isLogin":true,"mid":7,"uname":"target"}}`)
	}))
	t.Cleanup(api.Close)
	m.client.APIBase = api.URL
	// Deliberately stale: switching must query the old account rather than trust UI.
	m.room = &domain.Room{ID: 101, Live: false}
	return m, target, &stops, secrets
}

func TestAccountSwitchConfirmationCoversSavedAndQRAccounts(t *testing.T) {
	m, target, stops, _ := switchModel(t, "")
	state := &lifecycleOBS{}
	state.active.Store(true)
	endpoint := lifecycleServer(t, state)
	if err := m.obsClient.Connect(context.Background(), endpoint, ""); err != nil {
		t.Fatal(err)
	}
	if cmd := m.perform("account:" + target.UID); cmd != nil || m.mode != "confirm" {
		t.Fatal("saved account switched without confirmation")
	}
	m.modalKey(tea.KeyMsg{Type: tea.KeyEnter}) // default is cancel
	if m.store.Config().ActiveUID != "42" || stops.Load() != 0 || !state.active.Load() || state.stops.Load() != 0 {
		t.Fatal("cancel changed identity or broadcast")
	}
	m.mode = "qr"
	cmd := m.result(resultMsg{kind: "poll", value: domain.LoginPoll{Code: 0, Account: &target}})
	if cmd != nil || m.mode != "confirm" {
		t.Fatal("QR login bypassed switch confirmation")
	}
	m.selected = 1
	cmd = m.modalKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirmed login did not run")
	}
	result := cmd().(resultMsg)
	if result.err != nil {
		t.Fatal(result.err)
	}
	m.busy = false
	m.result(result)
	if m.account.UID != "7" || m.store.Config().ActiveUID != "7" || stops.Load() != 1 || state.active.Load() || state.stops.Load() != 1 {
		t.Fatal("confirmed switch did not stop old live/OBS streams and commit new identity")
	}
}

func TestAccountSwitchFailureRetainsFreshOldRoomAndIdentity(t *testing.T) {
	m, target, stops, _ := switchModel(t, "/room/v1/Room/stopLive")
	m.perform("account:" + target.UID)
	m.selected = 1
	result := m.modalKey(tea.KeyMsg{Type: tea.KeyEnter})().(resultMsg)
	if result.err == nil {
		t.Fatal("failed remote stop accepted")
	}
	m.busy = false
	m.result(result)
	if m.store.Config().ActiveUID != "42" || m.account.UID != "42" {
		t.Fatal("failed cleanup committed target identity")
	}
	if m.room == nil || !m.room.Live || m.room.ID != 202 || stops.Load() != 0 {
		t.Fatal("failure lost fresh old-account room state")
	}
}

func TestQueuedAccountSwitchCannotCommitAfterQuit(t *testing.T) {
	m, target, stops, _ := switchModel(t, "")
	m.perform("account:" + target.UID)
	m.selected = 1
	queued := m.modalKey(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	result := queued().(resultMsg)
	if result.err == nil || m.store.Config().ActiveUID != "42" || stops.Load() != 0 {
		t.Fatal("queued account switch escaped quit barrier")
	}
}

func TestAccountSwitchSaveFailureKeepsConfirmedStopVisible(t *testing.T) {
	m, target, stops, secrets := switchModel(t, "")
	secrets.reject = true
	m.perform("account:" + target.UID)
	m.selected = 1
	result := m.modalKey(tea.KeyMsg{Type: tea.KeyEnter})().(resultMsg)
	if result.err == nil {
		t.Fatal("credential failure accepted")
	}
	m.busy = false
	m.result(result)
	if m.account.UID != "42" || m.store.Config().ActiveUID != "42" {
		t.Fatal("failed save changed active identity")
	}
	if stops.Load() != 1 || m.room == nil || m.room.Live || m.room.ID != 202 {
		t.Fatal("confirmed old-account stop was lost after local save failure")
	}
}
