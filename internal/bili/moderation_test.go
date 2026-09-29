package bili

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"arcana-world/internal/domain"
)

func TestModerationScopeAndIndependentActions(t *testing.T) {
	posts := 0
	wantPath, wantHour := "", ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/xlive/app-blink/v1/room/GetInfo":
			fmt.Fprint(w, `{"code":0,"data":{"room_id":2}}`)
		case "/room/v1/Room/get_info":
			fmt.Fprint(w, `{"code":0,"data":{"uid":77,"room_id":200,"short_id":2}}`)
		default:
			posts++
			if r.URL.Path != wantPath || r.Method != http.MethodPost || r.Form.Get("tuid") != "42" || r.Form.Get("csrf") != "token" {
				t.Errorf("incorrect moderation action: %s %s %v", r.Method, r.URL.Path, r.Form)
			}
			if wantHour != "" {
				if r.Form.Get("hour") != wantHour || r.Form.Get("type") != "1" || r.Form.Get("room_id") != "2" {
					t.Errorf("incorrect mute duration or scope: %v", r.Form)
				}
			} else if r.URL.Path == "/xlive/web-ucenter/v1/banned/DelSilentUser" {
				if r.Form.Get("room_id") != "2" || r.Form.Has("anchor_id") {
					t.Errorf("unmute used blacklist scope: %v", r.Form)
				}
			} else if r.Form.Get("anchor_id") != "77" || r.Form.Get("spmid") != "444.8.0.0" || r.Form.Has("hour") {
				t.Errorf("blacklist used login UID or mute scope: %v", r.Form)
			}
			fmt.Fprint(w, `{"code":0,"data":null}`)
		}
	}))
	defer server.Close()
	client, err := New("direct", "")
	if err != nil {
		t.Fatal(err)
	}
	client.HTTP, client.LiveBase = server.Client(), server.URL
	client.SetAccount(domain.Account{UID: "1", Cookies: map[string]string{"DedeUserID": "1", "SESSDATA": "session", "bili_jct": "token"}})
	ctx := context.Background()
	// 开播请求即使被平台拒绝也可能写入房间缓存，不能将缓存当作归属证明。
	client.roomID = 999
	if err := client.AddRoomAdmin(ctx, 999, 42); err == nil || posts != 0 {
		t.Fatal("wrong-room appointment reached mutation endpoint")
	}
	actions := []struct {
		path string
		hour string
		run  func() error
	}{
		{"/xlive/app-ucenter/v2/xbanned/banned/AddBlack", "", func() error { return client.AddRoomBlock(ctx, 2, 42) }},
		{"/xlive/app-ucenter/v2/xbanned/banned/DelBlack", "", func() error { return client.RemoveRoomBlock(ctx, 2, 42) }},
		{"/xlive/web-ucenter/v1/banned/AddSilentUser", "3", func() error { return client.MuteRoomUser(ctx, 2, 42, 3) }},
		{"/xlive/web-ucenter/v1/banned/DelSilentUser", "", func() error { return client.RemoveRoomMute(ctx, 2, 42) }},
	}
	for _, action := range actions {
		wantPath, wantHour = action.path, action.hour
		before := posts
		if err := action.run(); err != nil || posts != before+1 {
			t.Fatalf("action %s: posts=%d, err=%v", action.path, posts-before, err)
		}
	}
}
