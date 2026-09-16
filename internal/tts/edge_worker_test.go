package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestEdgeRejectsUnsafeInput(t *testing.T) {
	for _, text := range []string{"", " \t\n", "hello\x00world", string([]byte{0xff}), strings.Repeat("中", 501)} {
		data, _ := json.Marshal(edgeRequest{Text: text, Voice: defaultVoice})
		// Invalid UTF-8 is replaced by encoding/json; exercise the public boundary directly.
		if text == string([]byte{0xff}) {
			edge := &Edge{timeout: time.Second}
			if _, err := edge.Synthesize(context.Background(), text); !errors.Is(err, ErrInvalidText) {
				t.Fatal(err)
			}
			continue
		}
		if _, err := readEdgeRequest(bytes.NewReader(data)); err == nil {
			t.Fatalf("accepted invalid text %q", text)
		}
	}
	for _, input := range []string{
		`{"text":"ok","voice":"zh-CN-XiaoxiaoNeural","extra":true}`,
		`{"text":"ok","voice":"zh-CN-XiaoxiaoNeural"} {}`,
		`{"text":"ok","voice":"zh-CN-XiaoxiaoNeural"} trailing`,
		`{"voice":"zh-CN-XiaoxiaoNeural"}`,
		`{"text":"ok","voice":"x' injected='Neural"}`,
		`{"text":"ok","voice":"zh-CN-XiaoxiaoNeural","proxy":"https://user:secret@example.com"}`,
		strings.Repeat(" ", (8<<10)+1),
	} {
		var out bytes.Buffer
		err := RunEdge(strings.NewReader(input), &out)
		if err == nil || out.Len() != 0 {
			t.Fatalf("accepted invalid request: %v", err)
		}
		if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "injected") {
			t.Fatal("input leaked")
		}
	}
	data, _ := json.Marshal(edgeRequest{Text: strings.Repeat("中", 500), Voice: defaultVoice})
	if _, err := readEdgeRequest(bytes.NewReader(data)); err != nil {
		t.Fatal(err)
	}
}
func TestProxyValidation(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"", ""}, {"http://user:password@localhost:8080", "http://user:password@localhost:8080"}, {"socks5h://127.0.0.1:1080", "socks5://127.0.0.1:1080"}, {"http://[::1]:8080", "http://[::1]:8080"},
	} {
		got, err := normalizeProxy(tc.input)
		if err != nil || got != tc.want {
			t.Fatalf("proxy normalization: %q %v", got, err)
		}
	}
	for _, input := range []string{"http://", "http://localhost:0", "http://localhost:65536", "http://localhost:", "http://localhost/path", "http://localhost?query", "http://localhost#fragment", "http://bad_host", "http://user:secret@%xx", "socks5://localhost", "socks5h://localhost"} {
		_, err := normalizeProxy(input)
		if err == nil {
			t.Fatalf("accepted %q", input)
		}
		if strings.Contains(err.Error(), input) || strings.Contains(err.Error(), "secret") {
			t.Fatal("proxy leaked")
		}
	}
	for _, input := range []string{"https://user:secret@localhost", "ftp://localhost"} {
		if _, err := normalizeProxy(input); !errors.Is(err, ErrUnsupportedProxy) {
			t.Fatal(err)
		}
	}
}
func TestProxyEnvironmentPrecedence(t *testing.T) {
	for _, tc := range []struct{ explicit, noProxy, want string }{
		{"", "", "http://environment.invalid:8080"}, {"direct", "", ""}, {"", "speech.platform.bing.com", ""}, {"socks5h://explicit.invalid:1080", "*", "socks5://explicit.invalid:1080"},
	} {
		env := append(withoutProxyEnv(fixtureEnv("proxy")), "HTTPS_PROXY=http://environment.invalid:8080", "NO_PROXY="+tc.noProxy, "ARCANA_TTS_TEST_PROXY="+tc.explicit)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		data, err := runHelper(ctx, testExecutable(t), nil, 2048, env)
		cancel()
		if err != nil || string(data) != tc.want {
			t.Fatalf("proxy precedence: got %q, %v; want %q", data, err, tc.want)
		}
	}
	env := withoutProxyEnv([]string{"http_proxy=a", "HtTpS_pRoXy=b", "ALL_PROXY=c", "no_proxy=d", "SAFE=value"})
	if len(env) != 1 || env[0] != "SAFE=value" {
		t.Fatalf("proxy environment retained: %v", env)
	}
}
func TestSafeEdgeErrors(t *testing.T) {
	for _, tc := range []struct {
		err  error
		want string
	}{
		{&url.Error{Op: "dial", URL: "http://user:secret@host/private-text", Err: context.DeadlineExceeded}, "timeout"},
		{websocket.ErrBadHandshake, "handshake rejected"},
		{errors.New("private-text secret"), "network or protocol failure"},
	} {
		err := safeEdgeError(tc.err)
		if !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private-text") {
			t.Fatal(err)
		}
	}
}
