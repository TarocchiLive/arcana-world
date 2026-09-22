package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestChatRecentWindowDoesNotBackfillOrPageBeyondLimit(t *testing.T) {
	m := chatTestModel(t)
	for n := 1; n <= 50; n++ {
		appendChat(t, m, fmt.Sprintf("message-%03d", n))
	}
	runChatCommand(m, m.readChat())
	body := m.chatView(100)
	if strings.Contains(body, "message-040") || !strings.Contains(body, "message-041") || !strings.Contains(body, "message-050") {
		t.Fatal("startup did not select the newest ten records")
	}
	runChatCommand(m, m.readChat())
	if strings.Contains(m.chatView(100), "message-040") {
		t.Fatal("refresh backfilled older records")
	}
	for n := 51; n <= 85; n++ {
		appendChat(t, m, fmt.Sprintf("message-%03d", n))
	}
	runChatCommand(m, m.readChat())
	m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	body = m.chatView(100)
	if strings.Contains(body, "message-055") || !strings.Contains(body, "message-056") || !strings.Contains(body, "message-085") {
		t.Fatal("scrolling exposed records outside the configured thirty-message window")
	}
	m.config.DanmakuLimit = 5
	runChatCommand(m, m.readChat())
	body = m.chatView(100)
	if strings.Contains(body, "message-080") || !strings.Contains(body, "message-081") {
		t.Fatal("smaller limit was not applied")
	}
}

func TestChatHistoryRangeDoesNotReplaceLiveWindowOrDraft(t *testing.T) {
	m := chatTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	for n := 1; n <= 50; n++ {
		appendChat(t, m, fmt.Sprintf("record-%03d", n))
	}
	runChatCommand(m, m.readChat())
	m.View()
	m.view.GotoTop()
	m.chatInput.SetValue("keep draft")
	m.openChatHistory()
	b := m.chat.historyBrowser
	b.inputs[0].SetValue(time.Now().Add(-time.Hour).Format(chatTimeFormat))
	b.inputs[1].SetValue(time.Now().Add(time.Hour).Format(chatTimeFormat))
	runChatCommand(m, m.readChatHistory())
	history := m.chatHistoryView()
	if !strings.Contains(history, "record-001") || !strings.Contains(history, "record-050") {
		t.Fatal("history was limited by main chat window")
	}
	appendChat(t, m, "live-arrival")
	runChatCommand(m, m.readChat())
	if strings.Contains(m.chatHistoryView(), "live-arrival") {
		t.Fatal("live refresh changed the independent history result")
	}
	m.closeChatHistory()
	m.View()
	if m.view.YOffset() != 0 || m.chatInput.Value() != "keep draft" || !strings.Contains(m.chatView(100), "live-arrival") {
		t.Fatal("returning lost the reader position, draft, or arriving messages")
	}
}

func TestChatScrollbarAndReaderPositionSurviveArrival(t *testing.T) {
	m := chatTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 18})
	for n := 1; n <= 10; n++ {
		appendChat(t, m, fmt.Sprintf("record-%03d", n))
	}
	runChatCommand(m, m.readChat())
	m.View()
	if !m.view.AtBottom() {
		t.Fatal("startup did not reveal newest record")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyHome})
	m.View()
	if m.view.AtBottom() {
		t.Fatal("short viewport cannot scroll back")
	}
	appendChat(t, m, "incoming-record")
	runChatCommand(m, m.readChat())
	screen := m.View().Content
	if m.view.YOffset() != 0 || !strings.Contains(screen, "┃") {
		t.Fatal("arrival moved the reader or scrollbar is missing")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if screen = m.View().Content; !m.view.AtBottom() || !strings.Contains(screen, "incoming-record") {
		t.Fatal("End did not reveal the latest record")
	}
}
