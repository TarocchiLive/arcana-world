package webui

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

// 防止恶意网页或未授权本机客户端触发直播控制，且不锁死合法会话。
func TestControlRequiresPrivateTokenAndSameOrigin(t *testing.T) {
	var mutations atomic.Int32
	server, err := Listen(0, func(_ context.Context, _ string, command *Command) (Snapshot, error) {
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
		name, token, host, origin, site string
		status                          int
	}{
		{name: "missing credential", status: http.StatusUnauthorized},
		{name: "incorrect credential", token: "incorrect", status: http.StatusUnauthorized},
		{name: "cross origin", token: server.token, origin: "https://attacker.invalid", status: http.StatusForbidden},
		{name: "rebound host", token: server.token, host: "attacker.invalid", status: http.StatusForbidden},
		{name: "cross site fetch", token: server.token, site: "cross-site", status: http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, base+"/api/command", strings.NewReader(`{"page":"studio","action":"start-confirm"}`))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "Bearer "+tc.token)
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
	request.Header.Set("Authorization", "Bearer "+server.token)
	request.Header.Set("Origin", base)
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || mutations.Load() != 1 {
		t.Fatalf("authorized control failed: status=%d mutations=%d", response.StatusCode, mutations.Load())
	}
}
