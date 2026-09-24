package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"arcana-world/internal/presentation"
)

func TestOverlayChatFollowsMainSnapshot(t *testing.T) {
	m := chatTestModel(t)
	m.overlayEnabled = true
	m.overlay = &overlayRuntime{state: "running"}
	m.config.DanmakuLimit = 1
	for n := range 15 {
		appendChat(t, m, fmt.Sprintf("record-%02d", n))
	}
	assertSnapshot := func() {
		t.Helper()
		want := ""
		if len(m.chat.entries) != 0 {
			want = presentation.RenderOverlay(m.chat.entries[0])
		}
		if got := m.overlayContentText(); got != want {
			t.Fatalf("overlay diverged from main snapshot: got %q, want %q", got, want)
		}
	}
	// 尚未加载主列表时，浮层不能自行补入历史。
	assertSnapshot()
	runChatCommand(m, m.readChat())
	assertSnapshot()
	appendChat(t, m, "new-message")
	// 存储已变化，但两端必须等待同一份快照。
	assertSnapshot()
	runChatCommand(m, m.readChat())
	assertSnapshot()
	m.config.OverlayDisabledEvents = []string{"chat"}
	if got := m.overlayContentText(); got != "" {
		t.Fatalf("filtered chat remained visible: %q", got)
	}
	m.config.OverlayDisabledEvents = nil
	assertSnapshot()
	m.overlayEnabled = false
	if got := m.overlayContentText(); got != "" {
		t.Fatalf("disabled overlay retained chat: %q", got)
	}
	m.overlayEnabled = true
	assertSnapshot()
	m.chat.entries = nil
	assertSnapshot()
}

func TestOverlayChatRemovesWithdrawnMessages(t *testing.T) {
	m := chatTestModel(t)
	m.overlayEnabled = true
	m.overlay = &overlayRuntime{state: "running"}
	appendRaw := func(raw string) {
		t.Helper()
		if _, err := m.chat.history.Append(1, json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
		runChatCommand(m, m.readChat())
	}
	appendRaw(`{"cmd":"SUPER_CHAT_MESSAGE","data":{"id":42,"uid":7,"price":30,"message":"withdraw-me","user_info":{"uname":"viewer"}}}`)
	if got := m.overlayContentText(); !strings.Contains(got, "withdraw-me") {
		t.Fatalf("SC missing before withdrawal: %q", got)
	}
	appendRaw(`{"cmd":"SUPER_CHAT_MESSAGE_DELETE","data":{"ids":[42]}}`)
	if got := m.overlayContentText(); strings.Contains(got, "withdraw-me") {
		t.Fatalf("withdrawn SC remained visible: %q", got)
	}
}
