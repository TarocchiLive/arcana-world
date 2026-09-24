package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"arcana-world/internal/app"
	"arcana-world/internal/bili"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"arcana-world/internal/obs"
)

func TestAudienceRejectsOldAccountAndClearsFailedRefresh(t *testing.T) {
	client := &bili.Client{}
	m := &Model{client: client, account: &domain.Account{UID: "1"}, room: &domain.Room{ID: 2}}
	m.applyAudience(audienceMsg{client: client, roomID: 2, uid: "1", count: 12})
	if got := m.audienceText(); got != fmt.Sprintf(i18n.T(i18n.TUIAudienceCount), 12) {
		t.Fatal(got)
	}
	m.account = &domain.Account{UID: "3"}
	m.applyAudience(audienceMsg{client: client, roomID: 2, uid: "1", count: 99})
	if got := m.audienceText(); got != i18n.T(i18n.TUIAudienceUnknown) {
		t.Fatalf("old account audience shown: %s", got)
	}
	m.applyAudience(audienceMsg{client: client, roomID: 2, uid: "3", count: 0})
	if got := m.audienceText(); got != fmt.Sprintf(i18n.T(i18n.TUIAudienceCount), 0) {
		t.Fatal(got)
	}
	m.applyAudience(audienceMsg{client: client, roomID: 2, uid: "3", err: errors.New("offline")})
	if got := m.audienceText(); got != i18n.T(i18n.TUIAudienceUnknown) {
		t.Fatalf("failed refresh retained stale audience: %s", got)
	}
}

func TestAudienceRejectsResponseAfterReturningToRoom(t *testing.T) {
	client := &bili.Client{}
	m := &Model{client: client, account: &domain.Account{UID: "1"}, room: &domain.Room{ID: 2}}
	m.updateAudience()
	old := audienceMsg{client: client, roomID: 2, uid: "1", generation: m.audience.generation, count: 99}
	m.room = &domain.Room{ID: 3}
	m.updateAudience()
	m.room = &domain.Room{ID: 2}
	m.updateAudience()
	m.applyAudience(audienceMsg{client: client, roomID: 2, uid: "1", generation: m.audience.generation, count: 12})
	m.applyAudience(old)
	if got := m.audienceText(); got != fmt.Sprintf(i18n.T(i18n.TUIAudienceCount), 12) {
		t.Fatalf("previous visit replaced current audience: %s", got)
	}
}

func TestBlockedAudienceDoesNotDelaySessionStop(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/xlive/general-interface/v1/rank/getOnlineGoldRank":
			close(started)
			<-r.Context().Done()
		case "/room/v1/Room/stopLive":
			close(stopped)
			fmt.Fprint(w, `{"code":0,"data":{}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client, err := bili.New("direct")
	if err != nil {
		t.Fatal(err)
	}
	defer client.HTTP.CloseIdleConnections()
	client.HTTP, client.LiveBase = server.Client(), server.URL
	account := domain.Account{UID: "1", Cookies: map[string]string{"SESSDATA": "session", "bili_jct": "csrf"}}
	client.SetAccount(account)
	session := app.New(nil, obs.NewClient(), client)
	m := &Model{ctx: ctx, client: client, session: session, account: &account, room: &domain.Room{ID: 2, Live: true}}
	command := m.updateAudience()
	if command == nil {
		t.Fatal("audience request was not scheduled")
	}
	audienceDone := make(chan audienceMsg, 1)
	go func() { audienceDone <- command().(audienceMsg) }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("audience request did not start")
	}
	// 人数响应始终阻塞，真实停播流程仍必须取得 gate 并发出请求。
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	if _, err := session.Stop(stopCtx, client, *m.room, domain.Config{}); err != nil {
		t.Fatalf("blocked audience delayed stop: %v", err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("stop returned without issuing the stop request")
	}
	// 根上下文取消后人数请求自行结束，不要求退出流程等待网络响应。
	cancel()
	select {
	case msg := <-audienceDone:
		if !errors.Is(msg.err, context.Canceled) {
			t.Fatalf("audience cancellation = %v", msg.err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("audience request ignored root cancellation")
	}
}
