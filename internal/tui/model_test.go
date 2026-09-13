package tui

import (
	"context"
	"os"
	"strings"
	"testing"

	"arcana-world/internal/domain"
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
	// The HTTP request finished, but its success message is still queued when Esc arrives.
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
