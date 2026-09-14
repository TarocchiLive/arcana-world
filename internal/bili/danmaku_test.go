// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
	"bytes"
	"compress/zlib"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/gorilla/websocket"
)

func compressDanmakuTest(t *testing.T, version uint16, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	if version == 2 {
		w := zlib.NewWriter(&b)
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		w := brotli.NewWriter(&b)
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return danmakuPacket(5, version, b.Bytes())
}
func TestDanmakuPackets(t *testing.T) {
	first := danmakuPacket(5, 0, []byte(`{"cmd":"one"}`))
	second := danmakuPacket(5, 1, []byte(`{"cmd":"two"}`))
	nested := compressDanmakuTest(t, 3, compressDanmakuTest(t, 2, append(first, second...)))
	var got []string
	budget := danmakuMaxExpanded
	err := danmakuDecode(append(nested, 1, 2), 0, &budget, func(op uint32, b []byte) error {
		if op != 5 {
			t.Fatalf("op %d", op)
		}
		got = append(got, string(b))
		return nil
	})
	if err == nil || len(got) != 2 || got[0] != `{"cmd":"one"}` || got[1] != `{"cmd":"two"}` {
		t.Fatalf("got %v, error %v", got, err)
	}
	sentinel := errors.New("storage failed")
	budget = danmakuMaxExpanded
	if err := danmakuDecode(nested, 0, &budget, func(uint32, []byte) error { return sentinel }); err != sentinel {
		t.Fatalf("consumer error changed: %v", err)
	}
}
func TestDanmakuPacketBounds(t *testing.T) {
	valid := danmakuPacket(5, 0, []byte(`{}`))
	for _, size := range []uint32{0, 15, uint32(len(valid) + 1), danmakuMaxPacket + 1} {
		data := append([]byte(nil), valid...)
		binary.BigEndian.PutUint32(data, size)
		budget := danmakuMaxExpanded
		if err := danmakuDecode(data, 0, &budget, func(uint32, []byte) error { t.Fatal("invalid packet delivered"); return nil }); err == nil {
			t.Fatalf("accepted length %d", size)
		}
	}
	data := compressDanmakuTest(t, 3, append(valid, valid...))
	budget := len(valid)
	count := 0
	if err := danmakuDecode(data, 0, &budget, func(uint32, []byte) error { count++; return nil }); err == nil || count != 1 {
		t.Fatalf("expansion bound: count=%d err=%v", count, err)
	}
	data = valid
	for i := 0; i <= danmakuMaxDepth; i++ {
		data = compressDanmakuTest(t, 2, data)
	}
	budget = danmakuMaxExpanded
	if err := danmakuDecode(data, 0, &budget, func(uint32, []byte) error { t.Fatal("excess nesting delivered"); return nil }); err == nil {
		t.Fatal("accepted excessive nesting")
	}
	// Sibling expansions share the same budget.
	data = append(compressDanmakuTest(t, 2, valid), compressDanmakuTest(t, 3, valid)...)
	budget = len(valid)
	count = 0
	if err := danmakuDecode(data, 0, &budget, func(uint32, []byte) error { count++; return nil }); err == nil || count != 1 {
		t.Fatalf("cumulative budget: count=%d err=%v", count, err)
	}
}
func TestDanmakuWBI(t *testing.T) {
	key, err := danmakuWBIKey("https://i0.hdslb.com/bfs/wbi/7cd084941338484aae1ad9425b84077c.png", "https://i0.hdslb.com/bfs/wbi/4932caff0ff746eab6f01bf08b70ac45.png")
	if err != nil || key != "ea1db124af3c7062474693fa704f4ff8" {
		t.Fatalf("key %q err %v", key, err)
	}
	query := url.Values{"foo": {"114"}, "bar": {"514"}, "baz": {"1919810"}}
	got := danmakuWBISign(query, key, time.Unix(1702204169, 0))
	// Independently calculated with Python urllib.parse.urlencode + hashlib.md5.
	if got.Get("w_rid") != "6149fdadf571698ca7e6a567265cd0ee" {
		t.Fatalf("signature %s", got.Get("w_rid"))
	}
	if query.Get("wts") != "" {
		t.Fatal("mutated input")
	}
	special := url.Values{"foo": {"a!'()* b~"}}
	got = danmakuWBISign(special, key, time.Unix(1, 0))
	if got.Get("foo") != "a b~" || special.Get("foo") != "a!'()* b~" {
		t.Fatal("filter mutated input or failed")
	}
}
func TestDanmakuHostAllowlist(t *testing.T) {
	for _, host := range []string{"evil.com", "chat.bilibili.com.evil.com", "a.chat.bilibili.com:443", "a.chat.bilibili.com/evil", ".chat.bilibili.com", "a@b.chat.bilibili.com"} {
		if danmakuHost(host) {
			t.Fatalf("accepted %q", host)
		}
	}
	if !danmakuHost("broadcastlv.chat.bilibili.com") {
		t.Fatal("rejected comet host")
	}
}

func danmakuTestConnection(t *testing.T, serve func(*websocket.Conn)) *websocket.Conn {
	t.Helper()
	finished := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		serve(conn)
	}))
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		conn.Close()
		server.Close()
		select {
		case <-finished:
		case <-time.After(time.Second):
			t.Error("server handler did not stop")
		}
	})
	return conn
}
func TestDanmakuAuthRejected(t *testing.T) {
	conn := danmakuTestConnection(t, func(c *websocket.Conn) {
		c.ReadMessage()
		c.WriteMessage(websocket.BinaryMessage, danmakuPacket(8, 1, []byte(`{"code":-101,"message":"secret"}`)))
	})
	called := false
	err := danmakuSession(context.Background(), conn, []byte(`{}`), func() { called = true }, func(json.RawMessage) error { return nil }, time.Second, time.Second)
	if err == nil || called || strings.Contains(err.Error(), "secret") {
		t.Fatalf("auth failure: called=%v err=%v", called, err)
	}
}
func TestDanmakuCancelPreservesCompletedFrame(t *testing.T) {
	conn := danmakuTestConnection(t, func(c *websocket.Conn) {
		c.ReadMessage()
		frame := danmakuPacket(8, 1, []byte(`{"code":0}`))
		frame = append(frame, danmakuPacket(5, 0, []byte(`{"cmd":"one"}`))...)
		frame = append(frame, danmakuPacket(5, 0, []byte(`{"cmd":"two"}`))...)
		c.WriteMessage(websocket.BinaryMessage, frame)
		c.ReadMessage()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sentinel := errors.New("disk full")
	count := 0
	err := danmakuSession(ctx, conn, []byte(`{}`), cancel, func(json.RawMessage) error {
		count++
		if count == 2 {
			return sentinel
		}
		return nil
	}, time.Second, time.Second)
	if err != sentinel || count != 2 {
		t.Fatalf("got count=%d err=%v", count, err)
	}
}
func TestDanmakuHeartbeatRequiredDespiteBusinessTraffic(t *testing.T) {
	heartbeat := make(chan struct{}, 1)
	conn := danmakuTestConnection(t, func(c *websocket.Conn) {
		c.ReadMessage()
		c.WriteMessage(websocket.BinaryMessage, danmakuPacket(8, 1, []byte(`{"code":0}`)))
		readerDone := make(chan struct{})
		go func() {
			defer close(readerDone)
			for {
				_, b, err := c.ReadMessage()
				if err != nil {
					return
				}
				if len(b) >= 16 && binary.BigEndian.Uint32(b[8:]) == 2 {
					select {
					case heartbeat <- struct{}{}:
					default:
					}
				}
			}
		}()
		defer func() { c.Close(); <-readerDone }()
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-readerDone:
				return
			case <-ticker.C:
				if c.WriteMessage(websocket.BinaryMessage, danmakuPacket(5, 0, []byte(`{"cmd":"live"}`))) != nil {
					return
				}
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	count := 0
	err := danmakuSession(ctx, conn, []byte(`{}`), nil, func(json.RawMessage) error { count++; return nil }, 30*time.Millisecond, 50*time.Millisecond)
	if err == nil || ctx.Err() != nil || count == 0 {
		t.Fatalf("missing ack did not terminate session: count=%d err=%v", count, err)
	}
	select {
	case <-heartbeat:
	default:
		t.Fatal("no heartbeat sent")
	}
}

func TestDanmakuCancelDuringProxyConnect(t *testing.T) {
	entered := make(chan struct{})
	finished := make(chan struct{})
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer close(finished)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		close(entered)
		// Never answer CONNECT: cancellation must interrupt the proxy handshake.
		var b [1]byte
		conn.Read(b[:])
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.Proxy = http.ProxyURL(proxyURL)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dialer, err := danmakuDialer(ctx, tr, "wss://broadcastlv.chat.bilibili.com/sub")
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		conn, _, err := dialer.DialContext(ctx, "wss://broadcastlv.chat.bilibili.com/sub", nil)
		if conn != nil {
			conn.Close()
		}
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("proxy not reached")
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("canceled handshake succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled handshake blocked")
	}
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("proxy socket remained open")
	}
}

func TestDanmakuUsesHTTP1WithHTTP2CapableTLS(t *testing.T) {
	for _, viaProxy := range []bool{false, true} {
		name := "direct"
		if viaProxy {
			name = "https-proxy"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.WriteMessage(websocket.TextMessage, []byte("connected"))
			}))
			server.EnableHTTP2 = true
			server.StartTLS()
			defer server.Close()
			roots := x509.NewCertPool()
			roots.AddCert(server.Certificate())
			tr := &http.Transport{TLSClientConfig: &tls.Config{
				RootCAs: roots, NextProtos: []string{"h2", "http/1.1"},
			}}
			if viaProxy {
				proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method != http.MethodConnect {
						http.Error(w, "CONNECT required", http.StatusMethodNotAllowed)
						return
					}
					r.Header.Set("Authorization", r.Header.Get("Proxy-Authorization"))
					username, password, ok := r.BasicAuth()
					if !ok || username != "proxy-user" || password != "" {
						http.Error(w, "proxy credentials required", http.StatusProxyAuthRequired)
						return
					}
					upstream, err := net.Dial("tcp", server.Listener.Addr().String())
					if err != nil {
						return
					}
					defer upstream.Close()
					conn, rw, err := w.(http.Hijacker).Hijack()
					if err != nil {
						return
					}
					defer conn.Close()
					_, _ = rw.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
					_ = rw.Flush()
					finished := make(chan struct{})
					go func() { defer close(finished); _, _ = io.Copy(upstream, conn) }()
					_, _ = io.Copy(conn, upstream)
					_ = conn.Close()
					<-finished
				}))
				proxy.EnableHTTP2 = true
				proxy.StartTLS()
				defer proxy.Close()
				roots.AddCert(proxy.Certificate())
				proxyURL, _ := url.Parse(proxy.URL)
				proxyURL.User = url.User("proxy-user")
				tr.Proxy = http.ProxyURL(proxyURL)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			target := "wss" + strings.TrimPrefix(server.URL, "https")
			dialer, err := danmakuDialer(ctx, tr, target)
			if err != nil {
				t.Fatal(err)
			}
			conn, _, err := dialer.DialContext(ctx, target, nil)
			if err != nil {
				t.Fatalf("HTTP/2-capable TLS prevented websocket connection: %v", err)
			}
			defer conn.Close()
			_, data, err := conn.ReadMessage()
			if err != nil || string(data) != "connected" {
				t.Fatalf("websocket message missing: %q, %v", data, err)
			}
		})
	}
}

func TestDanmakuProxyRejectsReasonlessStatusWithoutPanic(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = io.WriteString(conn, "HTTP/1.1 407\r\n\r\n")
	}))
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dialer, err := danmakuDialer(ctx, &http.Transport{Proxy: http.ProxyURL(proxyURL)}, "wss://example.com/sub")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("proxy rejection crashed the listener instead of returning an error: %v", p)
		}
	}()
	conn, _, err := dialer.DialContext(ctx, "wss://example.com/sub", nil)
	if conn != nil {
		conn.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "407") {
		t.Fatalf("proxy rejection did not report its status safely: %v", err)
	}
}

func TestDanmakuSOCKSUsesDefaultPort(t *testing.T) {
	destination := ""
	rejected := errors.New("test dial stopped")
	proxyURL, _ := url.Parse("socks5h://localhost")
	tr := &http.Transport{Proxy: http.ProxyURL(proxyURL), DialContext: func(_ context.Context, _ string, stringAddr string) (net.Conn, error) {
		destination = stringAddr
		return nil, rejected
	}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	dialer, err := danmakuDialer(ctx, tr, "wss://example.com/sub")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = dialer.DialContext(ctx, "wss://example.com/sub", nil)
	if !errors.Is(err, rejected) || destination != "localhost:1080" {
		t.Fatalf("SOCKS default destination = %q, error = %v", destination, err)
	}
}
