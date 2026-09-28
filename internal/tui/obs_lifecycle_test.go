package tui

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"arcana-world/internal/domain"
	"arcana-world/internal/store"
	"github.com/gorilla/websocket"
	"github.com/zalando/go-keyring"
)

type lifecycleSecrets map[string]string

func (s lifecycleSecrets) Get(key string) (string, error) {
	value, ok := s[key]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return value, nil
}
func (s lifecycleSecrets) Set(key, value string) error {
	s[key] = value
	return nil
}
func (s lifecycleSecrets) Delete(key string) error {
	delete(s, key)
	return nil
}
func (s lifecycleSecrets) Storage() store.StorageKind {
	return store.StorageUnknown
}

type lifecycleOBS struct {
	active       atomic.Bool
	stops        atomic.Int32
	rejectStop   bool
	started      chan struct{}
	releaseStart chan struct{}
	closed       chan struct{}
}

func lifecycleServer(t *testing.T, state *lifecycleOBS) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		if state.closed != nil {
			defer func() { state.closed <- struct{}{} }()
		}
		write := func(op int, data any) error { return ws.WriteJSON(map[string]any{"op": op, "d": data}) }
		if write(0, map[string]any{"rpcVersion": 1}) != nil {
			return
		}
		var envelope struct {
			Op int             `json:"op"`
			D  json.RawMessage `json:"d"`
		}
		if ws.ReadJSON(&envelope) != nil || envelope.Op != 1 {
			return
		}
		if write(2, map[string]any{"negotiatedRpcVersion": 1}) != nil {
			return
		}
		for {
			if ws.ReadJSON(&envelope) != nil {
				return
			}
			var request struct {
				ID   string `json:"requestId"`
				Type string `json:"requestType"`
			}
			if json.Unmarshal(envelope.D, &request) != nil {
				return
			}
			ok, code := true, 100
			switch request.Type {
			case "StartStream":
				state.active.Store(true)
				if state.started != nil {
					close(state.started)
					<-state.releaseStart
				}
			case "StopStream":
				state.stops.Add(1)
				if state.rejectStop {
					ok, code = false, 500
				} else {
					state.active.Store(false)
				}
			}
			if write(7, map[string]any{"requestId": request.ID, "requestType": request.Type, "requestStatus": map[string]any{"result": ok, "code": code}, "responseData": map[string]any{"outputActive": state.active.Load(), "outputReconnecting": false}}) != nil {
				return
			}
		}
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http")
}

func lifecycleModel(t *testing.T, ctx context.Context) *Model {
	t.Helper()
	s, err := store.OpenWithBackend(t.TempDir(), lifecycleSecrets{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(ctx, s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	return m
}

func awaitLifecycle(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatal("OBS lifecycle synchronization timed out")
	}
}

func TestCloseWaitsForCanceledStartBeforeStopping(t *testing.T) {
	state := &lifecycleOBS{started: make(chan struct{}), releaseStart: make(chan struct{})}
	endpoint := lifecycleServer(t, state)
	m := lifecycleModel(t, context.Background())
	if err := m.obsClient.Connect(context.Background(), endpoint, ""); err != nil {
		t.Fatal(err)
	}
	command := m.runOBS(startOperation().label, m.obsClient.Start)
	done := make(chan struct{})
	go func() { command(); close(done) }()
	awaitLifecycle(t, state.started)
	closed := make(chan error, 1)
	go func() { closed <- m.Close() }()
	awaitLifecycle(t, done)
	close(state.releaseStart)
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("close did not finish after start cancellation")
	}
	if state.active.Load() || state.stops.Load() != 1 {
		t.Fatal("in-flight start escaped shutdown stop")
	}
	if err := m.session.Lock(context.Background()); err == nil {
		t.Fatal("accepted OBS operation after shutdown")
	}
}

func TestStopLiveStopsOBSBeforeBilibiliAndPropagatesFailure(t *testing.T) {
	for _, reject := range []bool{false, true} {
		name := "success"
		if reject {
			name = "rejected"
		}
		t.Run(name, func(t *testing.T) {
			state := &lifecycleOBS{rejectStop: reject, closed: make(chan struct{}, 1)}
			state.active.Store(true)
			endpoint := lifecycleServer(t, state)
			var calls atomic.Int32
			live := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if state.active.Load() {
					t.Error("Bilibili Stop called before OBS stopped")
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
			}))
			defer live.Close()
			m := lifecycleModel(t, context.Background())
			cfg := m.store.Config()
			cfg.ExitLiveStopDisabled = true
			cfg.ExitOBSStopDisabled = !reject
			if err := m.store.SaveConfig(cfg); err != nil {
				t.Fatal(err)
			}
			account := domain.Account{Cookies: map[string]string{"SESSDATA": "session", "bili_jct": "csrf"}}
			m.account = &account
			m.client.SetAccount(account)
			m.client.LiveBase = live.URL
			m.room = &domain.Room{ID: 1, Live: true}
			if err := m.obsClient.Connect(context.Background(), endpoint, ""); err != nil {
				t.Fatal(err)
			}
			result := m.stopLive()().(taskMessage)
			if reject {
				if result.taskError() == nil || calls.Load() != 0 {
					t.Fatal("OBS failure was not propagated before Bilibili Stop")
				}
				if err := m.Close(); err == nil {
					t.Fatal("shutdown hid OBS stop failure")
				}
				awaitLifecycle(t, state.closed)
			} else if result.taskError() != nil || calls.Load() != 1 {
				t.Fatalf("stop live result: %v; Bilibili calls: %d", result.taskError(), calls.Load())
			}
		})
	}
}
