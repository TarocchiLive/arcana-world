package bili

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"arcana-world/internal/domain"
)

func TestModerationPaginationScopeAndMute(t *testing.T) {
	posts := 0
	wantHour := "-1"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		switch r.URL.Path {
		case "/xlive/app-blink/v1/room/GetInfo":
			fmt.Fprint(w, `{"code":0,"data":{"room_id":2}}`)
		case "/xlive/app-ucenter/v1/roomAdmin/get_by_anchor":
			page := r.Form.Get("page")
			fmt.Fprintf(w, `{"code":0,"data":{"data":[{"uid":%s,"uname":"admin"}],"page":{"total_page":2}}}`, page)
		case "/xlive/web-ucenter/v1/banned/GetSilentUserList":
			page := r.Form.Get("ps")
			fmt.Fprintf(w, `{"code":0,"data":{"data":[{"tuid":%s,"tname":"muted"}],"total_page":2}}`, page)
		case "/xlive/web-ucenter/v1/banned/AddSilentUser":
			posts++
			if r.Method != http.MethodPost || r.Form.Get("room_id") != "2" || r.Form.Get("tuid") != "42" || r.Form.Get("hour") != wantHour || r.Form.Get("csrf") != "token" {
				t.Errorf("incorrect room mute request: %v", r.Form)
			}
			fmt.Fprint(w, `{"code":403,"message":"rejected"}`)
		default:
			posts++
			t.Errorf("unexpected endpoint: %s", r.URL.Path)
			fmt.Fprint(w, `{"code":0,"data":{}}`)
		}
	}))
	defer server.Close()
	client, err := New("direct")
	if err != nil {
		t.Fatal(err)
	}
	client.HTTP, client.LiveBase = server.Client(), server.URL
	client.SetAccount(domain.Account{UID: "1", Cookies: map[string]string{"DedeUserID": "1", "SESSDATA": "session", "bili_jct": "token"}})
	ctx := context.Background()
	admins, err := client.RoomAdmins(ctx, 2)
	if err != nil || len(admins) != 2 || admins[0].UID != 1 || admins[1].UID != 2 {
		t.Fatalf("incomplete admins: %+v, %v", admins, err)
	}
	blocks, err := client.RoomBlocks(ctx, 2)
	if err != nil || len(blocks) != 2 || blocks[0].UID != 1 || blocks[1].UID != 2 {
		t.Fatalf("incomplete blocks: %+v, %v", blocks, err)
	}
	// 开播请求即使被平台拒绝也可能写入房间缓存，不能将缓存当作归属证明。
	client.roomID = 999
	if err := client.AddRoomAdmin(ctx, 999, 42); err == nil {
		t.Fatal("appointed admin in wrong room scope")
	}
	if posts != 0 {
		t.Fatal("wrong-room appointment reached mutation endpoint")
	}
	if err := client.AddRoomBlock(ctx, 2, 42); err == nil {
		t.Fatal("rejected mute reported success")
	}
	if posts != 1 {
		t.Fatal("mute request missing")
	}
	wantHour = "2"
	if err := client.MuteRoomUser(ctx, 2, 42); err == nil {
		t.Fatal("rejected temporary mute reported success")
	}
	if posts != 2 {
		t.Fatal("temporary mute request missing")
	}
}
