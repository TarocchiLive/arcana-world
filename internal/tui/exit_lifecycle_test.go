package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"arcana-world/internal/bili"
	"arcana-world/internal/domain"
	tea "charm.land/bubbletea/v2"
)

func TestQuitConfirmationCancelRestoresEditor(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.form("proxy", "代理", "http://localhost:8080", false)
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if m.mode != "confirm" || m.selected != 0 {
		t.Fatal("quit did not open with Cancel selected")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.mode != "form" || m.input.Value() != "http://localhost:8080" {
		t.Fatal("canceling quit discarded the editor")
	}
	cfg := m.config
	cfg.TUITheme = "paper"
	if result := m.saveConfig(cfg, false)().(taskMessage); result.taskError() != nil {
		t.Fatal("canceling quit closed the session")
	}
}

func TestQuitConfirmationDefersCompletedSettingsResult(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	cfg := m.config
	cfg.TUITheme = "paper"
	save := m.saveConfig(cfg, false)
	m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m.Update(save())
	if m.mode != "confirm" || m.confirmAction != "quit" {
		t.Fatal("background completion displaced quit confirmation")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.mode != "" || m.busy || m.config.TUITheme != "paper" {
		t.Fatal("canceling quit lost the completed settings result")
	}
}

// 此服务模拟真实的房间查询，包括必需的封面审核状态。
func exitLiveServer(t *testing.T, live bool, failure string, requests, stops *atomic.Int32) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == failure {
			_, _ = w.Write([]byte(`{"code":-1,"message":"SESSDATA-secret","data":{}}`))
			return
		}
		var data any = map[string]any{}
		switch r.URL.Path {
		case "/xlive/app-blink/v1/index/GetRoomPreLiveStatus":
		case "/xlive/app-blink/v1/preLive/PreLive":
			data = map[string]any{"cover": map[string]any{"auditStatus": 1}}
		case "/xlive/app-blink/v1/room/GetInfo":
			if r.URL.Query().Get("uId") != "42" {
				t.Error("room lookup used a different account")
			}
			status := 0
			if live {
				status = 1
			}
			data = map[string]any{"room_id": 202, "live_status": status}
		case "/xlive/app-blink/v1/room/AnnounceInfo":
		case "/room/v1/Room/stopLive":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("room_id") != "202" || r.Form.Get("csrf") != "csrf-secret" {
				t.Error("stop used stale room or wrong account")
			}
			stops.Add(1)
		default:
			t.Errorf("unexpected live request: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": data})
	}))
	t.Cleanup(server.Close)
	return server.URL
}

func exitAccount() domain.Account {
	return domain.Account{UID: "42", Name: "current", Cookies: map[string]string{"DedeUserID": "42", "SESSDATA": "SESSDATA-secret", "bili_jct": "csrf-secret"}}
}

func persistExitAccount(t *testing.T, m *Model, account domain.Account) {
	t.Helper()
	if err := m.store.Save(account); err != nil {
		t.Fatal(err)
	}
	cfg := m.store.Config()
	cfg.ActiveUID = account.UID
	if err := m.store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestExitSwitchesIndependentlyStopExternalStreams(t *testing.T) {
	for _, tc := range []struct {
		name            string
		obsOff, liveOff bool
	}{
		{"both", false, false},
		{"obs-only", false, true},
		{"live-only", true, false},
		{"neither", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &lifecycleOBS{closed: make(chan struct{}, 1)}
			state.active.Store(true)
			endpoint := lifecycleServer(t, state)
			var requests, stops atomic.Int32
			liveURL := exitLiveServer(t, true, "", &requests, &stops)
			ctx, cancel := context.WithCancel(context.Background())
			m := lifecycleModel(t, ctx)
			cfg := m.store.Config()
			cfg.ExitOBSStopDisabled, cfg.ExitLiveStopDisabled = tc.obsOff, tc.liveOff
			if err := m.store.SaveConfig(cfg); err != nil {
				t.Fatal(err)
			}
			account := exitAccount()
			persistExitAccount(t, m, account)
			m.account = &account
			m.client.SetAccount(account)
			m.client.LiveBase = liveURL
			// 绝不能使用之前账号留下的过期房间。
			m.room = &domain.Room{ID: 101, Live: false}
			if err := m.obsClient.Connect(ctx, endpoint, ""); err != nil {
				t.Fatal(err)
			}
			cancel()
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			awaitLifecycle(t, state.closed)
			if state.active.Load() != tc.obsOff || state.stops.Load() != boolCount(!tc.obsOff) {
				t.Fatal("OBS exit switch was not independent")
			}
			if stops.Load() != boolCount(!tc.liveOff) || (tc.liveOff && requests.Load() != 0) {
				t.Fatalf("live exit switch: requests=%d stops=%d", requests.Load(), stops.Load())
			}
		})
	}
}

func boolCount(value bool) int32 {
	if value {
		return 1
	}
	return 0
}

func TestExitRefreshesCommittedAccountAndSkipsOfflineOrAbsentAccount(t *testing.T) {
	for _, mode := range []string{"startup", "switched", "refreshed", "deleted", "offline", "no-account", "pending-login"} {
		t.Run(mode, func(t *testing.T) {
			var requests, stops atomic.Int32
			liveURL := exitLiveServer(t, mode != "offline", "", &requests, &stops)
			m := lifecycleModel(t, context.Background())
			// 此场景验证不接入 OBS 时的哔哩哔哩清理。
			cfg := m.store.Config()
			cfg.OBSAutoConnect, cfg.OBSAutoStream = false, false
			if err := m.store.SaveConfig(cfg); err != nil {
				t.Fatal(err)
			}
			m.config = cfg
			m.client.LiveBase = liveURL
			account := exitAccount()
			switch mode {
			case "startup", "switched", "refreshed", "deleted":
				persistExitAccount(t, m, account)
				if mode == "switched" {
					old := domain.Account{UID: "7", Cookies: map[string]string{"DedeUserID": "7"}}
					m.account = &old
					m.client.SetAccount(old)
				}
				if mode == "refreshed" {
					stale := exitAccount()
					stale.Cookies["bili_jct"] = "expired-csrf"
					m.account = &stale
					m.client.SetAccount(stale)
				}
				if mode == "deleted" {
					m.account = &account
					m.client.SetAccount(account)
					queued := m.perform("delete:" + account.UID)().(taskMessage)
					if queued.taskError() != nil {
						t.Fatal(queued.taskError())
					}
					// 删除账号会先关闭旧直播，再移除身份。
					if stops.Load() != 1 {
						t.Fatal("deletion did not stop old broadcast")
					}
					requests.Store(0)
				}
			case "offline":
				persistExitAccount(t, m, account)
				m.account = &account
				m.client.SetAccount(account)
			case "pending-login":
				m.pendingAccount = &account
			}
			m.room = &domain.Room{ID: 101, Live: true}
			if err := m.Close(); err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "startup", "switched", "refreshed":
				if stops.Load() != 1 {
					t.Fatal("committed account broadcast was not closed")
				}
			case "offline":
				if stops.Load() != 0 {
					t.Fatal("exit tried to close an already offline room")
				}
			default:
				if requests.Load() != 0 {
					t.Fatal("exit contacted Bilibili without a logged-in account")
				}
			}
		})
	}
}

func TestExitFailuresStillReleaseResourcesAndRunOtherCleanup(t *testing.T) {
	for _, tc := range []struct {
		name, failure string
		rejectOBS     bool
	}{
		{"obs-stop", "", true},
		{"live-status", "/xlive/app-blink/v1/room/GetInfo", false},
		{"live-stop", "/room/v1/Room/stopLive", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := &lifecycleOBS{rejectStop: tc.rejectOBS, closed: make(chan struct{}, 1)}
			state.active.Store(true)
			endpoint := lifecycleServer(t, state)
			var requests, stops atomic.Int32
			liveURL := exitLiveServer(t, true, tc.failure, &requests, &stops)
			m := lifecycleModel(t, context.Background())
			account := exitAccount()
			persistExitAccount(t, m, account)
			m.account = &account
			m.client.SetAccount(account)
			m.client.LiveBase = liveURL
			if err := m.obsClient.Connect(context.Background(), endpoint, ""); err != nil {
				t.Fatal(err)
			}
			err := m.Close()
			if err == nil {
				t.Fatal("exit hid cleanup failure")
			}
			if strings.Contains(err.Error(), "SESSDATA-secret") || strings.Contains(err.Error(), "csrf-secret") {
				t.Fatal("exit failure exposed account secrets")
			}
			before := requests.Load()
			if again := m.Close(); again != err || requests.Load() != before {
				t.Fatal("repeated exit reran cleanup or lost failure")
			}
			awaitLifecycle(t, state.closed)
			if state.stops.Load() != 1 || (tc.rejectOBS && stops.Load() != 1) {
				t.Fatal("cleanup failure prevented independent stop")
			}
		})
	}
}

func TestExitUsesCommittedProxyBeforeResultDelivery(t *testing.T) {
	var oldRequests atomic.Int32
	oldProxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldRequests.Add(1)
		http.Error(w, "old proxy unavailable", http.StatusBadGateway)
	}))
	defer oldProxy.Close()
	var requests, stops atomic.Int32
	newProxy := exitLiveServer(t, true, "", &requests, &stops)
	m := lifecycleModel(t, context.Background())
	persistExitAccount(t, m, exitAccount())
	client, err := bili.New(oldProxy.URL)
	if err != nil {
		t.Fatal(err)
	}
	m.client = client
	m.client.LiveBase = "http://live.invalid"
	m.config.Proxy = oldProxy.URL
	cfg := m.store.Config()
	cfg.Proxy = newProxy
	if err := m.store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if oldRequests.Load() != 0 || stops.Load() != 1 {
		t.Fatal("exit ignored the committed proxy while its UI result was queued")
	}
}
