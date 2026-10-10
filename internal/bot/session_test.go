package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"arcana-world/internal/bili"
	"arcana-world/internal/danmaku"
	"arcana-world/internal/domain"
)

type sessionSnapshot struct{ value danmaku.Snapshot }

func (s *sessionSnapshot) Snapshot() danmaku.Snapshot { return s.value }
func (*sessionSnapshot) Close() error                 { return nil }

// 防止来源身份变化或客户端覆盖房间后误发，也防止将远端拒绝误报为发送成功。
func TestSendMessagePinsIdentityAndReportsAPIResult(t *testing.T) {
	var calls atomic.Int32
	var reject atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		cookie, err := r.Cookie("SESSDATA")
		if r.URL.Path != "/msg/send" || r.Method != http.MethodPost || r.Form.Get("roomid") != "7" || err != nil || cookie.Value != "account-42" {
			t.Error("request used the wrong destination or identity")
		}
		if reject.Load() {
			fmt.Fprint(w, `{"code":0,"message":"private-backend-detail"}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"message":""}`)
	}))
	defer server.Close()
	client, err := bili.New("direct", "")
	if err != nil {
		t.Fatal(err)
	}
	defer client.HTTP.CloseIdleConnections()
	client.LiveBase = server.URL
	client.SetAccount(domain.Account{UID: "42", Cookies: map[string]string{"SESSDATA": "account-42", "bili_jct": "csrf-42"}})
	source := &sessionSnapshot{danmaku.Snapshot{AccountUID: "99", RoomID: 7, Phase: danmaku.PhaseConnected}}
	session := &Session{uid: "42", authentication: "authenticated", client: client, listener: source}
	call := func(params string) (bool, string, int) {
		t.Helper()
		var output protocolOutput
		request := `{"jsonrpc":"2.0","method":"send_message","params":` + params + `,"id":1}`
		if err := Serve(context.Background(), io.NopCloser(strings.NewReader(request)), &output, session); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output.String(), "private-backend-detail") || strings.Contains(output.String(), "account-42") {
			t.Fatal("response leaked private data")
		}
		var response struct {
			Result struct {
				Sent   bool   `json:"sent"`
				RoomID string `json:"room_id"`
			} `json:"result"`
			Error *Error `json:"error"`
		}
		if err := json.Unmarshal(output.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		code := 0
		if response.Error != nil {
			code = response.Error.Code
		}
		return response.Result.Sent, response.Result.RoomID, code
	}
	if sent, _, code := call(`{"text":"hello"}`); sent || code != -32002 || calls.Load() != 0 {
		t.Fatal("changed listener identity was allowed to send")
	}
	source.value.AccountUID = "42"
	if sent, _, code := call(`{"text":"hello","room_id":"8"}`); sent || code != -32602 || calls.Load() != 0 {
		t.Fatal("client destination override was allowed to send")
	}
	if sent, room, code := call(`{"text":"hello"}`); !sent || room != "7" || code != 0 || calls.Load() != 1 {
		t.Fatal("valid send did not report the actual API result")
	}
	reject.Store(true)
	if sent, _, code := call(`{"text":"hello"}`); sent || code != -32003 || calls.Load() != 2 {
		t.Fatal("API rejection reported success or caused a retry")
	}
}
