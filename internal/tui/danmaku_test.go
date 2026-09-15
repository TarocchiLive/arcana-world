package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"arcana-world/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func chatTestModel(t *testing.T) *Model {
	t.Helper()
	s, err := store.Open(t.TempDir())
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
	m.chat.room = 1
	return m
}
func appendChat(t *testing.T, m *Model, text string) {
	t.Helper()
	raw := json.RawMessage(fmt.Sprintf(`{"cmd":"DANMU_MSG","info":[[],%q,[1,"viewer"]]}`, text))
	if _, err := m.chat.history.Append(1, raw); err != nil {
		t.Fatal(err)
	}
}
func runChatCommand(m *Model, cmd tea.Cmd) {
	for cmd != nil {
		switch msg := cmd().(type) {
		case chatPageMsg:
			cmd = m.applyChatPage(msg)
		case resultMsg:
			m.busy = false
			cmd = m.result(msg)
		default:
			return
		}
	}
}
func chatKeyRun(m *Model, key string) { _, cmd := m.chatKey(key); runChatCommand(m, cmd) }

func TestChatHistoryCursorsStayStableWhileMessagesArrive(t *testing.T) {
	m := chatTestModel(t)
	for i := 1; i <= 205; i++ {
		appendChat(t, m, fmt.Sprintf("event-%03d", i))
	}
	runChatCommand(m, m.readChat())
	if m.chat.entries[0].Text != "event-205" {
		t.Fatal("latest history not loaded")
	}
	chatKeyRun(m, "[")
	if m.chat.entries[0].Text != "event-105" || m.chat.entries[99].Text != "event-006" {
		t.Fatal("older page lost its exclusive sequence boundary")
	}
	appendChat(t, m, "event-206")
	runChatCommand(m, m.readChat())
	if m.chat.entries[0].Text != "event-105" {
		t.Fatal("new messages shifted frozen history")
	}
	chatKeyRun(m, "]")
	if m.chat.entries[0].Text != "event-205" {
		t.Fatal("returning to newer page jumped to live data")
	}
	chatKeyRun(m, "end")
	if m.chat.entries[0].Text != "event-206" {
		t.Fatal("return-to-live failed")
	}
	chatKeyRun(m, "[")
	chatKeyRun(m, "[")
	chatKeyRun(m, "[")
	if m.chat.entries[0].Text != "event-006" || m.chat.entries[len(m.chat.entries)-1].Text != "event-001" {
		t.Fatal("paging past oldest erased history or broke back navigation")
	}
}

func TestChatTogglePersistsWithoutDeletingHistory(t *testing.T) {
	m := chatTestModel(t)
	appendChat(t, m, "keep this history")
	chatKeyRun(m, "s")
	if m.mode != "confirm" || m.config.DanmakuDisabled {
		t.Fatal("disabling must explain offline gaps before changing setting")
	}
	runChatCommand(m, m.perform("chat-disable"))
	if !m.store.Config().DanmakuDisabled {
		t.Fatal("disabled state was not persisted")
	}
	reopened, err := store.Open(m.store.Dir())
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.Config().DanmakuDisabled {
		t.Fatal("disabled state did not survive reopening")
	}
	runChatCommand(m, m.readChat())
	if m.chat.entries[0].Text != "keep this history" {
		t.Fatal("toggle removed saved messages")
	}
	m.mode = ""
	chatKeyRun(m, "s")
	if m.store.Config().DanmakuDisabled {
		t.Fatal("listener could not be re-enabled")
	}
}

func TestChatDeletionUpdatesPausedHistoryAndStripsTerminalControls(t *testing.T) {
	m := chatTestModel(t)
	for _, raw := range []string{`{"cmd":"SUPER_CHAT_MESSAGE","data":{"id":42,"uid":7,"price":30,"message":"withdraw-me","user_info":{"uname":"viewer"}}}`, `{"cmd":"DANMU_MSG","info":[[],"safe\u001b[2J\u0007",[1,"viewer"]]}`} {
		if _, err := m.chat.history.Append(1, json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
	}
	runChatCommand(m, m.readChat())
	chatKeyRun(m, " ")
	if _, err := m.chat.history.Append(1, json.RawMessage(`{"cmd":"SUPER_CHAT_MESSAGE_DELETE","data":{"ids":[42]}}`)); err != nil {
		t.Fatal(err)
	}
	runChatCommand(m, m.readChat())
	rendered := ""
	for _, e := range m.chat.entries {
		rendered += chatEventText(e)
	}
	if strings.Contains(rendered, "withdraw-me") || strings.ContainsAny(rendered, "\x1b\a") {
		t.Fatal("deleted content or active terminal control escaped into history view")
	}
	records, err := m.chat.history.Page(1, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatal("display pause or deletion discarded the audit event")
	}
}

func TestChatDisplayBoundsPreserveStoredText(t *testing.T) {
	m := chatTestModel(t)
	text := strings.Repeat("界", 10_000) + "\nfull-message-ending"
	appendChat(t, m, text)
	runChatCommand(m, m.readChat())
	rendered := chatEventText(m.chat.entries[0])
	if len(rendered) > 20_000 || strings.Contains(rendered, "full-message-ending") || strings.Contains(rendered, "\n") {
		t.Fatal("oversized input escaped the bounded single-record display")
	}
	records, err := m.chat.history.Page(1, 0, 1)
	if err != nil || records[0].Text != text {
		t.Fatal("display truncation modified durable message content")
	}
}

func TestChatPauseRejectsAnInflightLivePage(t *testing.T) {
	m := chatTestModel(t)
	appendChat(t, m, "already displayed")
	runChatCommand(m, m.readChat())
	appendChat(t, m, "arrived during refresh")
	pending := m.readChat()
	queued := pending().(chatPageMsg)
	chatKeyRun(m, " ")
	runChatCommand(m, m.applyChatPage(queued))
	if m.chat.follow || len(m.chat.entries) != 1 || m.chat.entries[0].Text != "already displayed" {
		t.Fatal("in-flight history refresh defeated the user's pause")
	}
}

func TestChatScrollingDoesNotHideIncomingMessages(t *testing.T) {
	m := chatTestModel(t)
	runChatCommand(m, m.readChat())
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	appendChat(t, m, "arrived after scrolling")
	runChatCommand(m, m.readChat())
	if !strings.Contains(m.chatView(), "arrived after scrolling") {
		t.Fatal("scrolling an empty page hid incoming chat")
	}
	appendChat(t, m, "arrived during refresh")
	pending := m.readChat()
	queued := pending().(chatPageMsg)
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	runChatCommand(m, m.applyChatPage(queued))
	if !strings.Contains(m.chatView(), "arrived during refresh") {
		t.Fatal("scrolling discarded an incoming chat refresh")
	}
}

func TestChatDetailFieldsCannotInjectTerminalControlsOrUnboundedText(t *testing.T) {
	m := chatTestModel(t)
	value := "\x1b]52;c;payload\a\n" + strings.Repeat("x", 20000)
	encoded, _ := json.Marshal(value)
	raw := fmt.Sprintf(`{"cmd":"ROOM_SILENT_ON","data":{"type":%s,"second":-1}}`, encoded)
	if _, err := m.chat.history.Append(1, json.RawMessage(raw)); err != nil {
		t.Fatal(err)
	}
	runChatCommand(m, m.readChat())
	line := chatEventText(m.chat.entries[0])
	if strings.ContainsAny(line, "\x1b\a\r\n") || len(line) > 4096 {
		t.Fatal("protocol detail escaped the bounded single-line renderer")
	}
	if !strings.Contains(line, "second=-1") {
		t.Fatal("long future enum hid the following semantic field")
	}
}

func TestChatAllowlistCannotBeBypassedByOtherToggle(t *testing.T) {
	m := chatTestModel(t)
	raws := []string{
		`{"cmd":"DANMU_MSG","info":[[],"allowed-chat",[1,"viewer"]]}`,
		`{"cmd":"ENTRY_EFFECT","data":{"uid":1,"copy_writing":"optional-entry-effect"}}`,
		`{"cmd":"USER_TOAST_MSG","data":{"username":"viewer","toast_msg":"allowed-guard-purchase"}}`,
		`{"cmd":"PK_INVITE_INIT","uname":"optional-pk-invite","invite_id":1}`,
		`{"cmd":"PK_BATTLE_RANK_CHANGE","data":{"rank_name":"optional-pk-rank"}}`,
		`{"cmd":"NOTICE_MSG","msg_common":"forbidden-broadcast"}`,
		`{"cmd":"GUARD_MSG","msg":"forbidden-cross-room-guard","buy_type":3}`,
		`{"cmd":"ONLINE_RANK_V2","data":{"online_list":[{"uname":"forbidden-ranking"}]}}`,
		`{"cmd":"ONLINE_RANK_COUNT","data":{"count":424242}}`,
		`{"cmd":"FUTURE_UNLISTED_EVENT","data":{"text":"forbidden-future"}}`,
	}
	for _, raw := range raws {
		if _, err := m.chat.history.Append(1, json.RawMessage(raw)); err != nil {
			t.Fatal(err)
		}
	}
	runChatCommand(m, m.readChat())
	for _, other := range []bool{false, true, false} {
		if m.chat.showOther != other {
			chatKeyRun(m, "f")
		}
		rendered := m.chatView()
		if !strings.Contains(rendered, "allowed-chat") || !strings.Contains(rendered, "allowed-guard-purchase") {
			t.Fatal("essential room activity was hidden")
		}
		if strings.Contains(rendered, "optional-entry-effect") != other {
			t.Fatal("Other must only toggle optional allowlisted room activity")
		}
		if strings.Contains(rendered, "optional-pk-invite") != other ||
			strings.Contains(rendered, "optional-pk-rank") != other {
			t.Fatal("PK invitations and PK ranks must follow Other without exposing non-PK rankings")
		}
		for _, hidden := range []string{"forbidden-broadcast", "forbidden-cross-room-guard", "forbidden-ranking", "424242", "FUTURE_UNLISTED_EVENT", "forbidden-future"} {
			if strings.Contains(rendered, hidden) {
				t.Fatalf("Other=%v exposed hidden event %q", other, hidden)
			}
		}
	}
	stored, err := m.chat.history.Page(1, 0, 100)
	if err != nil || len(stored) != len(raws) {
		t.Fatalf("display filtering changed retained history: count=%d err=%v", len(stored), err)
	}
}
