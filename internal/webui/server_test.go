package webui

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"arcana-world/internal/broadcast"

	"github.com/coder/websocket"
)

// 防止恶意网页触发直播控制，同时允许无需凭证的同源访问。
func TestControlRequiresSameOrigin(t *testing.T) {
	var mutations atomic.Int32
	server, err := Listen("127.0.0.1", 0, func(_ context.Context, _ string, command *Command) (Snapshot, error) {
		if command != nil {
			mutations.Add(1)
		}
		return Snapshot{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = server.Serve() }()
	t.Cleanup(func() { _ = server.Close(context.Background()) })
	base := "http://" + server.host
	client := &http.Client{}
	for _, tc := range []struct {
		name, host, origin, site string
		status                   int
	}{
		{name: "cross origin", origin: "https://attacker.invalid", status: http.StatusForbidden},
		{name: "malformed origin", origin: base + ":80", status: http.StatusForbidden},
		{name: "rebound host", host: "attacker.invalid", status: http.StatusForbidden},
		{name: "cross site fetch", site: "cross-site", status: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, base+"/api/command", strings.NewReader(`{"page":"studio","action":"start-confirm"}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			if tc.host != "" {
				request.Host = tc.host
			}
			if tc.origin != "" {
				request.Header.Set("Origin", tc.origin)
			}
			if tc.site != "" {
				request.Header.Set("Sec-Fetch-Site", tc.site)
			}
			response, err := client.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			_, _ = io.Copy(io.Discard, response.Body)
			if response.StatusCode != tc.status || mutations.Load() != 0 {
				t.Fatalf("status=%d, unauthorized mutations=%d", response.StatusCode, mutations.Load())
			}
		})
	}
	request, err := http.NewRequest(http.MethodPost, base+"/api/command", strings.NewReader(`{"page":"studio","action":"start-confirm"}`))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", base)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || mutations.Load() != 1 {
		t.Fatalf("same-origin control failed: status=%d mutations=%d", response.StatusCode, mutations.Load())
	}
}

// 共享广播允许跨源读取，但不能放宽控制接口权限或接管 WebUI 生命周期。
func TestSharedBroadcastSecurityBoundary(t *testing.T) {
	var mutations atomic.Int32
	server, err := Listen("127.0.0.1", 0, func(_ context.Context, _ string, command *Command) (Snapshot, error) {
		if command != nil {
			mutations.Add(1)
		}
		return Snapshot{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	stream := broadcast.New()
	server.SetBroadcast(stream)
	go func() { _ = server.Serve() }()
	t.Cleanup(func() {
		_ = stream.Close()
		_ = server.Close(context.Background())
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	origin := "https://attacker.invalid"
	conn, _, err := websocket.Dial(ctx, "ws://"+server.Addr()+"/ws", &websocket.DialOptions{
		HTTPHeader: http.Header{"Origin": {origin}, "Sec-Fetch-Site": {"cross-site"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.CloseNow()
	type message struct {
		kind websocket.MessageType
		data []byte
		err  error
	}
	received := make(chan message, 1)
	go func() {
		kind, data, err := conn.Read(ctx)
		received <- message{kind, data, err}
	}()
	// 等待服务端开始消费控制帧，避免无历史广播与连接注册之间的竞争。
	if err := conn.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	stream.Publish(42, json.RawMessage(`{"cmd":"DANMU_MSG","info":["hello"]}`))
	result := <-received
	if result.err != nil || result.kind != websocket.MessageText || string(result.data) != `{"room_id":42,"event":{"cmd":"DANMU_MSG","info":["hello"]}}` {
		t.Fatalf("broadcast kind=%v data=%s err=%v", result.kind, result.data, result.err)
	}
	for _, path := range []string{"/api/command", "/ws/"} {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+server.Addr()+path, strings.NewReader(`{"page":"studio","action":"start-confirm"}`))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Origin", origin)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, response.Body)
		_ = response.Body.Close()
		if response.StatusCode != http.StatusForbidden || mutations.Load() != 0 {
			t.Fatalf("path=%s status=%d unauthorized mutations=%d", path, response.StatusCode, mutations.Load())
		}
	}
	server.SetBroadcast(nil)
	if err := stream.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.Read(ctx); err == nil || ctx.Err() != nil {
		t.Fatalf("detached broadcast did not disconnect promptly: read=%v context=%v", err, ctx.Err())
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+server.Addr()+"/api/state?page=studio", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("WebUI stopped with broadcast: status=%d", response.StatusCode)
	}
}

// 用连接目的地址模拟局域网访问，不依赖测试机器拥有特定网卡。
func TestWildcardHostMustMatchLocalIPv4(t *testing.T) {
	server, err := Listen("0.0.0.0", 0, func(context.Context, string, *Command) (Snapshot, error) {
		return Snapshot{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.listener.Close() })
	port := server.listener.Addr().(*net.TCPAddr).Port
	local := &net.TCPAddr{IP: net.ParseIP("192.168.1.10"), Port: port}
	host := net.JoinHostPort(local.IP.String(), strconv.Itoa(port))
	for _, tc := range []struct {
		name, host, origin, contentType string
		status                          int
	}{
		{name: "LAN same origin", host: host, origin: "http://" + host, contentType: "application/json", status: http.StatusOK},
		{name: "DNS rebound", host: net.JoinHostPort("attacker.invalid", strconv.Itoa(port)), status: http.StatusForbidden},
		{name: "different local address", host: server.host, status: http.StatusForbidden},
		{name: "wrong port", host: net.JoinHostPort(local.IP.String(), strconv.Itoa(port+1)), status: http.StatusForbidden},
		{name: "LAN cross origin", host: host, origin: server.URL(), status: http.StatusForbidden},
		{name: "JSON required", host: host, origin: "http://" + host, contentType: "text/plain", status: http.StatusUnsupportedMediaType},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "http://"+host+"/api/command", strings.NewReader(`{"page":"studio","action":"start-confirm"}`))
			request.Host = tc.host
			request.Header.Set("Origin", tc.origin)
			request.Header.Set("Content-Type", tc.contentType)
			request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, local))
			response := httptest.NewRecorder()
			server.serveHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status=%d, want %d", response.Code, tc.status)
			}
			if response.Header().Get("X-Frame-Options") != "DENY" {
				t.Fatal("missing frame protection")
			}
		})
	}
}

func TestListenRejectsInvalidBind(t *testing.T) {
	for _, address := range []string{"localhost", "192.168.1.10", "::1", " 0.0.0.0", "127.0.0.2"} {
		t.Run(address, func(t *testing.T) {
			server, err := Listen(address, 0, nil)
			if err == nil {
				_ = server.listener.Close()
				t.Fatal("invalid bind address accepted")
			}
		})
	}
	server, err := Listen("", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer server.listener.Close()
	if address := server.listener.Addr().(*net.TCPAddr); !address.IP.Equal(net.ParseIP("127.0.0.1")) {
		t.Fatalf("default bind address=%s", address)
	}
	if strings.Contains(server.URL(), "#") {
		t.Fatalf("unexpected URL fragment: %s", server.URL())
	}
}

// 图片接口的较大请求上限不能扩大普通命令权限或绕过同源检查。
func TestCoverRequestBoundary(t *testing.T) {
	calls := 0
	server, err := Listen("127.0.0.1", 0, func(context.Context, string, *Command) (Snapshot, error) {
		calls++
		return Snapshot{}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.listener.Close() })
	for _, tc := range []struct {
		name, path, action, origin string
		size, status               int
	}{
		{name: "cover accepts larger JSON", path: "/api/cover", action: "cover-upload", size: 128 << 10, status: http.StatusOK},
		{name: "cover size limit", path: "/api/cover", action: "cover-upload", size: 2 << 20, status: http.StatusBadRequest},
		{name: "command size unchanged", path: "/api/command", action: "submit", size: 64 << 10, status: http.StatusBadRequest},
		{name: "cover action only", path: "/api/cover", action: "confirm", status: http.StatusBadRequest},
		{name: "cover endpoint required", path: "/api/command", action: "cover-upload", status: http.StatusBadRequest},
		{name: "cover same origin required", path: "/api/cover", action: "cover-upload", origin: "https://attacker.invalid", status: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := calls
			body := `{"page":"studio","action":"` + tc.action + `","value":"` + strings.Repeat("a", tc.size) + `"}`
			request := httptest.NewRequest(http.MethodPost, "http://"+server.host+tc.path, strings.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Origin", tc.origin)
			response := httptest.NewRecorder()
			server.serveHTTP(response, request)
			if response.Code != tc.status {
				t.Fatalf("status=%d, want %d", response.Code, tc.status)
			}
			wantCalls := before
			if tc.status == http.StatusOK {
				wantCalls++
			}
			if calls != wantCalls {
				t.Fatalf("backend calls=%d, want %d", calls, wantCalls)
			}
		})
	}
}
