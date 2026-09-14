package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"arcana-world/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func TestCanceledQRDoesNotReopenFromQueuedSuccess(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	command := m.work("qr", func(context.Context) (any, error) {
		return domain.QR{Key: "queued-key", URL: "https://passport.bilibili.com/login?token=queued-secret"}, nil
	})
	// HTTP 请求已完成，但 Esc 到达时成功消息仍在队列中。
	queued := command()
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	_, next := m.Update(queued)
	if strings.Contains(m.View(), "queued-secret") || next != nil {
		t.Fatal("cancelled QR success reopened login and resumed polling")
	}
}

func TestDiskJournalRedactsAccountAndStreamSecrets(t *testing.T) {
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	m.account = &domain.Account{Cookies: map[string]string{"SESSDATA": "SESSION_CANARY_38f7", "bili_jct": "CSRF_CANARY_c28a"}}
	m.stream = &domain.Stream{Key: "STREAM_CANARY_f71e"}
	m.log("operation-marker SESSION_CANARY_38f7 CSRF_CANARY_c28a STREAM_CANARY_f71e")
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(m.journal.Path())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "operation-marker") {
		t.Fatal("operation was not persisted")
	}
	for _, secret := range []string{"SESSION_CANARY_38f7", "CSRF_CANARY_c28a", "STREAM_CANARY_f71e"} {
		if strings.Contains(text, secret) {
			t.Fatal("credential reached disk journal")
		}
	}
}

func TestEnglishUIKeepsCredentialsHiddenAndUserTitlesIntact(t *testing.T) {
	if err := i18n.SetLanguage("en"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = i18n.SetLanguage("zh-CN") })
	s, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	m.account = &domain.Account{Name: "主播", Cookies: map[string]string{"SESSDATA": "SESSION_CANARY"}}
	m.room = &domain.Room{ID: 1, Title: "我的直播"}
	m.stream = &domain.Stream{Address: "rtmp://stream.example/live", Key: "STREAM_CANARY"}
	view := m.View()
	if !strings.Contains(view, "Live dashboard") || !strings.Contains(view, "我的直播") {
		t.Fatalf("English dashboard lost its localized heading or user title: %q", view)
	}
	if strings.Contains(view, "STREAM_CANARY") || strings.Contains(view, "SESSION_CANARY") {
		t.Fatal("English dashboard exposed credentials")
	}
	m.perform("reveal")
	if !strings.Contains(m.View(), "STREAM_CANARY") {
		t.Fatal("explicit reveal did not show stream credentials")
	}
	m.perform("reveal")
	if strings.Contains(m.View(), "STREAM_CANARY") {
		t.Fatal("hiding credentials did not redact the English dashboard")
	}
	m.perform("title")
	if !strings.Contains(m.selection.View(100, 10), "我的直播") {
		t.Fatal("English title editor translated user content")
	}
	result := selectionKey(m.selection, tea.KeyEnter)
	if result == nil || result.Title != "我的直播" {
		t.Fatalf("English title editor changed the submitted title: %+v", result)
	}
	m.log("SESSION_CANARY STREAM_CANARY")
	if strings.Contains(m.status, "SESSION_CANARY") || strings.Contains(m.status, "STREAM_CANARY") || !strings.Contains(m.status, "[hidden]") {
		t.Fatal("English log redaction failed")
	}
}
