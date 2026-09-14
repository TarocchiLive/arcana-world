package obs

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
	"github.com/gorilla/websocket"
)

func obsServer(t *testing.T, serve func(*websocket.Conn)) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer ws.Close()
		serve(ws)
	}))
	t.Cleanup(server.Close)
	return "ws" + strings.TrimPrefix(server.URL, "http")
}
func serverIdentify(ws *websocket.Conn) bool {
	if writeMessage(ws, 0, map[string]any{"rpcVersion": 1}) != nil {
		return false
	}
	var e envelope
	if ws.ReadJSON(&e) != nil || e.Op != 1 {
		return false
	}
	return writeMessage(ws, 2, map[string]any{"negotiatedRpcVersion": 1}) == nil
}
func waitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("server synchronization timed out")
	}
}
func TestPersistentSessionSurvivesConnectContext(t *testing.T) {
	var handshakes atomic.Int32
	endpoint := obsServer(t, func(ws *websocket.Conn) {
		handshakes.Add(1)
		if !serverIdentify(ws) {
			return
		}
		for {
			var e envelope
			if ws.ReadJSON(&e) != nil {
				return
			}
			var req struct {
				ID   string `json:"requestId"`
				Type string `json:"requestType"`
			}
			if json.Unmarshal(e.D, &req) != nil {
				return
			}
			if writeMessage(ws, 7, map[string]any{"requestId": req.ID, "requestType": req.Type, "requestStatus": map[string]any{"result": true, "code": 100}, "responseData": map[string]any{"outputActive": false, "outputReconnecting": false, "outputTimecode": "00:00:00.000"}}) != nil {
				return
			}
		}
	})
	c := NewClient()
	defer c.Close()
	ctx, cancel := context.WithCancel(context.Background())
	if err := c.Connect(ctx, endpoint, ""); err != nil {
		t.Fatal(err)
	}
	cancel()
	for range 2 {
		if _, err := c.Status(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := c.Connect(context.Background(), endpoint, ""); err != nil {
			t.Fatal(err)
		}
	}
	if handshakes.Load() != 1 {
		t.Fatalf("opened %d sessions", handshakes.Load())
	}
}
func TestConcurrentConnectAndDisconnectHandshake(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var handshakes atomic.Int32
	endpoint := obsServer(t, func(ws *websocket.Conn) {
		if handshakes.Add(1) == 1 {
			close(entered)
		}
		<-release
		serverIdentify(ws)
	})
	c := NewClient()
	defer c.Close()
	result := make(chan error, 1)
	go func() { result <- c.Connect(context.Background(), endpoint, "") }()
	waitSignal(t, entered)
	if err := c.Connect(context.Background(), endpoint, ""); err == nil {
		t.Error("concurrent handshake must return busy")
	}
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-result:
		if err == nil {
			t.Error("disconnected handshake succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("disconnect did not cancel handshake")
	}
	if c.Snapshot().Connected || c.Snapshot().Connecting {
		t.Fatal("late handshake resurrected connection")
	}
	if handshakes.Load() != 1 {
		t.Fatalf("opened %d sessions", handshakes.Load())
	}
}
func TestStreamEventsAndConfigureGuard(t *testing.T) {
	sendEvent := make(chan struct{})
	var configured atomic.Bool
	endpoint := obsServer(t, func(ws *websocket.Conn) {
		if !serverIdentify(ws) {
			return
		}
		<-sendEvent
		if writeMessage(ws, 5, map[string]any{"eventType": "StreamStateChanged", "eventData": map[string]any{"outputActive": false, "outputState": "OBS_WEBSOCKET_OUTPUT_RECONNECTING"}}) != nil {
			return
		}
		for {
			var e envelope
			if ws.ReadJSON(&e) != nil {
				return
			}
			var req struct {
				ID   string `json:"requestId"`
				Type string `json:"requestType"`
			}
			if json.Unmarshal(e.D, &req) != nil {
				return
			}
			if req.Type == "SetStreamServiceSettings" {
				configured.Store(true)
			}
			if writeMessage(ws, 7, map[string]any{"requestId": req.ID, "requestType": req.Type, "requestStatus": map[string]any{"result": true, "code": 100}, "responseData": map[string]any{"outputActive": false, "outputReconnecting": true}}) != nil {
				return
			}
		}
	})
	c := NewClient()
	defer c.Close()
	if err := c.Connect(context.Background(), endpoint, ""); err != nil {
		t.Fatal(err)
	}
	close(sendEvent)
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for !c.Snapshot().Status.Reconnecting {
		select {
		case <-c.Events():
		case <-timer.C:
			t.Fatal("stream event did not update snapshot")
		}
	}
	if err := c.Configure(context.Background(), domain.Stream{Address: "rtmp://example.test/live", Key: "secret"}); err == nil {
		t.Fatal("configured reconnecting stream")
	}
	if configured.Load() {
		t.Fatal("sent stream settings while reconnecting")
	}
}
func TestDisconnectReleasesPendingRequest(t *testing.T) {
	requested := make(chan struct{})
	endpoint := obsServer(t, func(ws *websocket.Conn) {
		if !serverIdentify(ws) {
			return
		}
		var e envelope
		if ws.ReadJSON(&e) != nil {
			return
		}
		close(requested)
		ws.ReadJSON(&e)
	})
	c := NewClient()
	defer c.Close()
	if err := c.Connect(context.Background(), endpoint, ""); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := c.Status(context.Background()); result <- err }()
	waitSignal(t, requested)
	c.Disconnect()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("pending request succeeded without reply")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pending request remained blocked")
	}
}
