package tui

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"arcana-world/internal/app"
	"arcana-world/internal/bili"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"arcana-world/internal/obs"
	"arcana-world/internal/store"
)

func TestAudienceRejectsOldAccountAndClearsFailedRefresh(t *testing.T) {
	client := &bili.Client{}
	m := &Model{client: client, account: &domain.Account{UID: "1"}, room: &domain.Room{ID: 2}}
	m.applyAudience(audienceMsg{client: client, roomID: 2, uid: "1", count: 12})
	if got := m.audienceText(); got != fmt.Sprintf(i18n.T(i18n.TUIAudienceCount), 11) {
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
	if got := m.audienceText(); got != fmt.Sprintf(i18n.T(i18n.TUIAudienceCount), 11) {
		t.Fatalf("previous visit replaced current audience: %s", got)
	}
}

func TestAudienceSelfFilterPreservesRawListAndFleet(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.account = &domain.Account{UID: "123"}
	t.Cleanup(func() { m.account = nil })
	m.room = &domain.Room{ID: 456}
	m.config.ActiveUID = "123"
	m.members = &roomMembersState{
		client: m.client, accountUID: "123", roomID: 456, loaded: true,
		list: bili.RoomMemberList{Total: 1, Members: []bili.RoomMember{{UID: 123, Name: "self-viewer", Rank: 1, Self: true}}},
	}
	m.applyAudience(audienceMsg{client: m.client, roomID: 456, uid: "123", count: 1})
	m.view.SetWidth(100)
	if got := m.roomMembersView(); strings.Contains(got, "self-viewer") || !strings.Contains(got, i18n.T(i18n.MembersEmpty)) {
		t.Fatalf("self-only list was not empty: %s", got)
	}
	if got := m.audienceText(); got != fmt.Sprintf(i18n.T(i18n.TUIAudienceCount), 0) {
		t.Fatal(got)
	}
	m.members.list.Members = append(m.members.list.Members, bili.RoomMember{UID: 789, Name: "other-viewer", Rank: 2})
	m.members.list.Total = 2
	if got := m.roomMembersView(); strings.Contains(got, "self-viewer") || !strings.Contains(got, "other-viewer") || !strings.Contains(got, fmt.Sprintf(i18n.T(i18n.MembersTotal), 1)) {
		t.Fatalf("wrong filtered list: %s", got)
	}
	m.members.fleet = true
	if got := m.roomMembersView(); !strings.Contains(got, "self-viewer") || !strings.Contains(got, fmt.Sprintf(i18n.T(i18n.FleetTotal), 2)) {
		t.Fatalf("fleet was filtered: %s", got)
	}
	m.members.fleet = false
	for _, include := range []bool{true, false} {
		msg := m.perform("audience-ignore-self")()
		if result, ok := msg.(taskMessage); !ok || result.taskError() != nil {
			t.Fatalf("toggle failed: %+v", msg)
		}
		m.Update(msg)
		reopened, err := store.OpenWithBackend(m.store.Dir(), lifecycleSecrets{})
		if err != nil {
			t.Fatal(err)
		}
		if reopened.Config().AudienceIncludeSelf != include {
			t.Fatal("toggle did not persist")
		}
		if got := m.roomMembersView(); strings.Contains(got, "self-viewer") != include {
			t.Fatalf("cached list did not follow toggle: %s", got)
		}
		wantCount := int64(0)
		if include {
			wantCount = 1
		}
		if got := m.audienceText(); got != fmt.Sprintf(i18n.T(i18n.TUIAudienceCount), wantCount) {
			t.Fatalf("cached count did not follow toggle: %s", got)
		}
	}
}

func TestAudienceHidesAnonymousSelfWithoutHidingOtherAnonymousViewers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"data":{"onlineNum":2,"OnlineRankItem":[{"uid":123,"name":"anonymous-self","userRank":1,"score":0,"is_mystery":true},{"uid":789,"name":"anonymous-other","userRank":2,"score":0,"is_mystery":true}]}}`)
	}))
	defer server.Close()
	m := lifecycleModel(t, context.Background())
	m.account = &domain.Account{UID: "123"}
	t.Cleanup(func() { m.account = nil })
	m.client.SetAccount(*m.account)
	m.client.HTTP, m.client.LiveBase = server.Client(), server.URL
	m.room = &domain.Room{ID: 456}
	m.config.ActiveUID = "123"
	list, err := m.client.RoomMembers(context.Background(), 456)
	if err != nil {
		t.Fatal(err)
	}
	m.members = &roomMembersState{client: m.client, accountUID: "123", roomID: 456, loaded: true, list: list}
	m.view.SetWidth(100)
	if got := m.roomMembersView(); strings.Contains(got, "anonymous-self") || !strings.Contains(got, "anonymous-other") {
		t.Fatalf("anonymous self filtering failed: %s", got)
	}
	m.config.AudienceIncludeSelf = true
	if got := m.roomMembersView(); !strings.Contains(got, "anonymous-self") || strings.Contains(got, "UID 123") || strings.Contains(got, "UID 789") {
		t.Fatalf("restored anonymous roster exposed identities: %s", got)
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
