package tui

import (
	"context"
	"strings"
	"testing"

	"arcana-world/internal/domain"
	"arcana-world/internal/store"
	tea "charm.land/bubbletea/v2"
)

func TestCanceledQRDoesNotReopenFromQueuedSuccess(t *testing.T) {
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Close()
	command := work(m, qrOperation(), func(context.Context) (domain.QR, error) {
		return domain.QR{Key: "queued-key", URL: "https://passport.bilibili.com/login?token=queued-secret"}, nil
	})
	// HTTP 请求已完成，但 Esc 到达时成功消息仍在队列中。
	queued := command()
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	_, next := m.Update(queued)
	if strings.Contains(m.View().Content, "queued-secret") || next != nil {
		t.Fatal("cancelled QR success reopened login and resumed polling")
	}
}
