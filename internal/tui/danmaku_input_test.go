package tui

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"arcana-world/internal/domain"
	tea "charm.land/bubbletea/v2"
)

func TestChatComposerOwnsKeyboardUntilBlurred(t *testing.T) {
	m := chatTestModel(t)
	m.Update(mouseRuneKey('/'))
	if !m.chatInput.Focused() {
		t.Fatal("/ did not focus the chat composer")
	}
	other, disabled := m.config.DanmakuShowOther, m.config.DanmakuDisabled
	for _, r := range "q2fs " {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
		if m.page != chatPage || m.mode != "" || m.exitPrompt != nil || !m.chatInput.Focused() {
			t.Fatalf("typing %q escaped the composer", r)
		}
		if m.config.DanmakuShowOther != other || m.config.DanmakuDisabled != disabled {
			t.Fatalf("typing %q activated a chat command", r)
		}
	}
	if got := m.chatInput.Value(); got != "q2fs " {
		t.Fatalf("draft = %q, want typed shortcut characters", got)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m.Update(tea.KeyPressMsg{Code: 'X', Text: "X"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	m.Update(tea.KeyPressMsg{Code: 'Y', Text: "Y"})
	if got := m.chatInput.Value(); got != "q2fsX Y" || m.page != chatPage {
		t.Fatalf("arrow keys navigated away instead of editing: page=%d draft=%q", m.page, got)
	}
	for _, key := range []rune{tea.KeyEsc, tea.KeyTab} {
		m.Update(tea.KeyPressMsg{Code: key})
		if m.chatInput.Focused() || m.page != chatPage || m.chatInput.Value() != "q2fsX Y" {
			t.Fatalf("blur key %v lost the draft or changed page", key)
		}
		m.Update(mouseRuneKey('/'))
		if !m.chatInput.Focused() || m.chatInput.Value() != "q2fsX Y" {
			t.Fatal("refocusing did not preserve the draft")
		}
	}
}

func TestChatComposerRetainsFailedSendAndClearsSuccessfulRetry(t *testing.T) {
	m := chatTestModel(t)
	account := domain.Account{UID: "42", Cookies: map[string]string{"SESSDATA": "session", "bili_jct": "csrf"}}
	m.account = &account
	m.client.SetAccount(account)
	m.room = &domain.Room{ID: 123}
	text := strings.Repeat("界", 30)
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
	m.Update(mouseRuneKey('/'))
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

func TestChatComposerLimitsCharactersRatherThanTerminalCells(t *testing.T) {
	m := chatTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 48, Height: 22})
	m.Update(mouseRuneKey('/'))
	m.Update(tea.PasteMsg{Content: strings.Repeat("界", 30) + "不能发送"})
	if got := m.chatInput.Value(); got != strings.Repeat("界", 30) {
		t.Fatalf("paste exceeded character limit or counted CJK cells: %q", got)
	}
	if m.workspace().chatComposer < 2 {
		t.Fatal("long CJK draft did not wrap in a narrow composer")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m.Update(mouseRuneKey('好'))
	if got := m.chatInput.Value(); got != strings.Repeat("界", 29)+"好" {
		t.Fatalf("editing at the character limit lost text: %q", got)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
	m.Update(mouseRuneKey('新'))
	if got := m.chatInput.Value(); got != strings.Repeat("界", 29)+"好" {
		t.Fatalf("inserting at the limit silently replaced the draft suffix: %q", got)
	}
	m.chatInput.SelectAll()
	m.Update(tea.PasteMsg{Content: strings.Repeat("文", 35)})
	if got := m.chatInput.Value(); got != strings.Repeat("文", 30) {
		t.Fatalf("replacing selected CJK text escaped the character limit: %q", got)
	}
}
