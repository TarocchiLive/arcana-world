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
	connections  atomic.Int32
	rejectStop   bool
	started      chan struct{}
	releaseStart chan struct{}
	drop         chan struct{}
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
		connection := state.connections.Add(1)
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
			if state.drop != nil && connection == 1 && request.Type == "GetStreamStatus" {
				<-state.drop
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

func TestCloseStopsOBSAfterRootCancellation(t *testing.T) {
	state := &lifecycleOBS{closed: make(chan struct{}, 1)}
	state.active.Store(true)
	endpoint := lifecycleServer(t, state)
	ctx, cancel := context.WithCancel(context.Background())
	m := lifecycleModel(t, ctx)
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
	if state.active.Load() || state.stops.Load() != 1 {
		t.Fatal("exit did not stop OBS exactly once before closing")
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

func TestCloseRecoversKnownActiveDisconnectedOBS(t *testing.T) {
	state := &lifecycleOBS{drop: make(chan struct{})}
	state.active.Store(true)
	endpoint := lifecycleServer(t, state)
	m := lifecycleModel(t, context.Background())
	cfg := m.store.Config()
	cfg.OBSURL = endpoint
	if err := m.store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.obsClient.Connect(context.Background(), endpoint, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := m.obsClient.Status(context.Background()); err != nil {
		t.Fatal(err)
	}
	close(state.drop)
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for m.obsClient.Snapshot().Connected {
		select {
		case <-m.obsClient.Events():
		case <-timer.C:
			t.Fatal("server disconnect was not observed")
		}
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if state.active.Load() || state.connections.Load() != 2 || state.stops.Load() != 1 {
		t.Fatal("lost active connection was not recovered and stopped")
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

func TestCloseReconnectsDisconnectedAutoStreamWhileLive(t *testing.T) {
	state := &lifecycleOBS{}
	state.active.Store(true)
	endpoint := lifecycleServer(t, state)
	m := lifecycleModel(t, context.Background())
	m.room = &domain.Room{ID: 1, Live: true}
	cfg := m.store.Config()
	cfg.OBSURL = endpoint
	cfg.OBSAutoStream = true
	if err := m.store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if state.active.Load() || state.connections.Load() != 1 || state.stops.Load() != 1 {
		t.Fatal("auto-stream exit skipped disconnected OBS")
	}
}

func TestCloseAvoidsUnnecessaryOBSRequests(t *testing.T) {
	t.Run("uncontrolled", func(t *testing.T) {
		state := &lifecycleOBS{}
		endpoint := lifecycleServer(t, state)
		m := lifecycleModel(t, context.Background())
		cfg := m.store.Config()
		cfg.OBSURL = endpoint
		if err := m.store.SaveConfig(cfg); err != nil {
			t.Fatal(err)
		}
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
		if state.connections.Load() != 0 {
			t.Fatal("exit connected to unused OBS")
		}
	})
	t.Run("inactive", func(t *testing.T) {
		state := &lifecycleOBS{}
		endpoint := lifecycleServer(t, state)
		m := lifecycleModel(t, context.Background())
		if err := m.obsClient.Connect(context.Background(), endpoint, ""); err != nil {
			t.Fatal(err)
		}
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
		if state.stops.Load() != 0 {
			t.Fatal("exit sent StopStream to inactive OBS")
		}
	})
}

func TestCloseSkipsDisconnectedAutoStreamBeforeGoingLive(t *testing.T) {
	for _, knownRoom := range []bool{false, true} {
		name := "no-room"
		if knownRoom {
			name = "offline-room"
		}
		t.Run(name, func(t *testing.T) {
			var attempts atomic.Int32
			unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				http.Error(w, "OBS unavailable", http.StatusServiceUnavailable)
			}))
			defer unavailable.Close()
			m := lifecycleModel(t, context.Background())
			cfg := m.store.Config()
			cfg.OBSURL = "ws" + strings.TrimPrefix(unavailable.URL, "http")
			cfg.OBSAutoStream = true
			if err := m.store.SaveConfig(cfg); err != nil {
				t.Fatal(err)
			}
			if knownRoom {
				m.room = &domain.Room{ID: 1}
			}
			if err := m.Close(); err != nil {
				t.Fatalf("offline exit reported a streaming risk: %v", err)
			}
			if attempts.Load() != 0 {
				t.Fatal("offline exit attempted to connect to unused OBS")
			}
		})
	}
}

func TestCloseReportsDisconnectedOBSWhileAutoStreamIsLive(t *testing.T) {
	var attempts atomic.Int32
	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		http.Error(w, "OBS unavailable", http.StatusServiceUnavailable)
	}))
	defer unavailable.Close()
	m := lifecycleModel(t, context.Background())
	cfg := m.store.Config()
	cfg.OBSURL = "ws" + strings.TrimPrefix(unavailable.URL, "http")
	cfg.OBSAutoStream = true
	if err := m.store.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	m.room = &domain.Room{ID: 1, Live: true}
	if err := m.Close(); err == nil {
		t.Fatal("live exit hid the failure to stop disconnected OBS")
	}
	if attempts.Load() != 1 {
		t.Fatal("live exit did not try to regain OBS control")
	}
}
