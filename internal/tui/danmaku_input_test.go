package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"arcana-world/internal/domain"
	"arcana-world/internal/store"
	tea "charm.land/bubbletea/v2"
)

func chatTestModel(t *testing.T) *Model {
	t.Helper()
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := m.Close(); err != nil {
			t.Error(err)
		}
	})
	if m.chat.err != nil {
		t.Fatal(m.chat.err)
	}
	m.page = chatPage
	return m
}

func TestChatComposerRetainsFailedSendAndClearsSuccessfulRetry(t *testing.T) {
	m := chatTestModel(t)
	account := domain.Account{UID: "42", Cookies: map[string]string{"SESSDATA": "session", "bili_jct": "csrf"}}
	m.account = &account
	m.client.SetAccount(account)
	m.room = &domain.Room{ID: 123}
	text := strings.Repeat("界", 40)
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/msg/send" {
			t.Errorf("unexpected send request: %s %s", r.Method, r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		if r.Form.Get("roomid") != "123" || r.Form.Get("msg") != text {
			t.Errorf("send used wrong room or draft: %v", r.Form)
		}
		if attempts.Add(1) == 1 {
			fmt.Fprint(w, `{"code":-400,"message":"rejected"}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{}}`)
	}))
	t.Cleanup(server.Close)
	m.client.LiveBase = server.URL
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.Update(tea.PasteMsg{Content: text})
	for attempt := 1; attempt <= 2; attempt++ {
		_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if cmd == nil {
			t.Fatal("Enter did not start a send")
		}
		result, ok := cmd().(taskMessage)
		if !ok {
			t.Fatal("send did not return a task result")
		}
		m.Update(result)
		want := text
		if attempt == 2 {
			want = ""
		}
		if got := m.chatInput.Value(); got != want {
			t.Fatalf("draft after attempt %d = %q, want %q", attempt, got, want)
		}
	}
	if got := attempts.Load(); got != 2 {
		t.Fatalf("send requests = %d, want 2", got)
	}
}
