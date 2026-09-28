package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"arcana-world/internal/domain"
)

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
