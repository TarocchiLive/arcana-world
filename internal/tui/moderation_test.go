package tui

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"arcana-world/internal/bili"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
)

type moderationTransport func(*http.Request) (*http.Response, error)

func (f moderationTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAvatarFailureDoesNotPreventConfirmedUnmute(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.config.ActiveUID = "1"
	m.config.DanmakuDisabled = true
	m.config.ExitLiveStopDisabled = true
	m.config.ExitOBSStopDisabled = true
	if err := m.store.SaveConfig(m.config); err != nil {
		t.Fatal(err)
	}
	m.account = &domain.Account{UID: "1", Cookies: map[string]string{"DedeUserID": "1", "SESSDATA": "session", "bili_jct": "csrf"}}
	m.client.SetAccount(*m.account)
	m.room = &domain.Room{ID: 2}
	var mutations atomic.Int32
	m.client.HTTP.Transport = moderationTransport(func(r *http.Request) (*http.Response, error) {
		status, body := http.StatusOK, `{"code":0,"data":{}}`
		switch r.URL.Path {
		case "/avatar.png":
			status, body = http.StatusServiceUnavailable, "avatar unavailable"
		case "/xlive/web-ucenter/v1/banned/DelSilentUser":
			if err := r.ParseForm(); err != nil {
				t.Error(err)
			}
			if r.Form.Get("tuid") != "7" || r.Form.Get("room_id") != "2" {
				t.Errorf("wrong unmute target: %v", r.Form)
			}
			mutations.Add(1)
		case "/xlive/web-ucenter/v1/banned/GetSilentUserList":
			body = `{"code":0,"data":{"data":[],"total_page":1}}`
		default:
			t.Errorf("unexpected request: %s", r.URL)
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header), Request: r}, nil
	})
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m.moderation = &moderationState{client: m.client, account: "1", roomID: 2, kind: moderationBlocks, action: moderationRemoveBlock,
		user: bili.RoomUser{UID: 7, Name: "target", Face: "https://i0.hdslb.com/avatar.png"}}
	m.selected, m.confirmAction = 1, "moderation-apply"
	if cmd := m.applyModeration(); cmd != nil {
		t.Fatal("mutation started without displaying a confirmation")
	}
	runChatCommand(m, m.loadModerationAvatar())
	if m.mode != "confirm" || m.confirmAction != "moderation-apply" || !strings.Contains(m.prompt, "UID") || !strings.Contains(m.prompt, i18n.T(i18n.ModerationAvatarMissing)) {
		t.Fatalf("identity confirmation lost after CDN failure: mode=%q prompt=%q", m.mode, m.prompt)
	}
	if m.selected != 0 || mutations.Load() != 0 {
		t.Fatal("avatar failure must not bypass explicit confirmation")
	}
	m.modalKey(tea.KeyPressMsg{Code: tea.KeyDown})
	runChatCommand(m, m.modalKey(tea.KeyPressMsg{Code: tea.KeyEnter}))
	if mutations.Load() != 1 {
		t.Fatalf("confirmed unmute requests = %d", mutations.Load())
	}
	if cmd := m.applyModeration(); cmd != nil {
		t.Fatal("the same confirmation authorized another mutation")
	}
}
