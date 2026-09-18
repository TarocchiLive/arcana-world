package tui

import (
	"strings"
	"testing"

	"arcana-world/internal/domain"
	"arcana-world/internal/store"
)

func TestOverlayChatClearsSourceBeforeAcceptingDelayedResults(t *testing.T) {
	m := &Model{
		config:         store.DefaultConfig(),
		account:        &domain.Account{UID: "first"},
		overlayEnabled: true,
		overlay:        &overlayRuntime{state: "running"},
	}
	m.syncOverlayChatSource()
	m.overlayChat.loading = true
	previous := overlayChatMsg{request: m.overlayChat.request, count: 1, lines: [overlayChatLimit]string{"first-account-only"}}
	m.handleOverlayChat(previous)
	if got := m.overlayContentText(); got != "first-account-only" {
		t.Fatalf("initial chat not shown: %q", got)
	}

	// 旧历史请求尚未返回时切换账号。
	m.account = &domain.Account{UID: "second"}
	if got := m.overlayContentText(); got != "" {
		t.Fatalf("old account remained visible during switch: %q", got)
	}
	m.overlayChat.loading = true
	m.handleOverlayChat(previous)
	if got := m.overlayContentText(); got != "" {
		t.Fatalf("late result restored old account text: %q", got)
	}
	current := overlayChatMsg{request: m.overlayChat.request, count: 1, lines: [overlayChatLimit]string{"second-account-only"}}
	m.handleOverlayChat(current)
	if got := m.overlayContentText(); got != "second-account-only" {
		t.Fatalf("current account result was lost: %q", got)
	}

	m.config.DanmakuDisabled = true
	if got := m.overlayContentText(); got != "" {
		t.Fatalf("disabled chat remained visible: %q", got)
	}
	m.handleOverlayChat(current)
	if got := m.overlayContentText(); strings.Contains(got, "account-only") {
		t.Fatalf("late result bypassed chat disable: %q", got)
	}
}
