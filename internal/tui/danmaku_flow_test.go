package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"arcana-world/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
)

func TestChatChronologyAndPinnedControlsFollowTheBottom(t *testing.T) {
	m := chatTestModel(t)
	for n := 1; n <= 100; n++ {
		appendChat(t, m, fmt.Sprintf("message-%03d", n))
	}
	runChatCommand(m, m.readChat())
	body := m.chatView()
	if strings.Index(body, "message-001") >= strings.Index(body, "message-100") {
		t.Fatal("chat does not progress from older to newer")
	}
	screen := m.View()
	if !m.view.AtBottom() || !strings.Contains(screen, "message-100") {
		t.Fatal("following did not reveal the newest message at the bottom")
	}
	if strings.Contains(body, i18n.T(i18n.DanmakuOther)) || !strings.Contains(screen, i18n.T(i18n.DanmakuOther)) {
		t.Fatal("Other control must stay outside the scrolling message body")
	}
	appendChat(t, m, "newest-live-arrival")
	runChatCommand(m, m.readChat())
	if screen := m.View(); !m.view.AtBottom() || !strings.Contains(screen, "newest-live-arrival") {
		t.Fatal("live arrival did not advance the bottom")
	}
}

func TestChatPausedAndPagedReadersSeeNewMessageBanner(t *testing.T) {
	for _, pause := range []string{" ", "["} {
		t.Run(fmt.Sprintf("pause-%q", pause), func(t *testing.T) {
			m := chatTestModel(t)
			for n := range 120 {
				appendChat(t, m, fmt.Sprintf("existing-%03d", n))
			}
			runChatCommand(m, m.readChat())
			m.View()
			chatKeyRun(m, pause)
			m.View()
			m.view.SetYOffset(3)
			offset, frozen := m.view.YOffset, m.chatView()
			banner := i18n.T(i18n.DanmakuNewMessages)
			if !strings.Contains(m.View(), banner) {
				t.Fatal("messages below the viewport need a scroll hint")
			}
			appendChat(t, m, "unseen-arrival")
			runChatCommand(m, m.readChat())
			screen := m.View()
			if !strings.Contains(screen, banner) || strings.Contains(screen, "unseen-arrival") {
				t.Fatal("paused reader needs a banner, not a replaced history page")
			}
			if m.chatView() != frozen || m.view.YOffset != offset {
				t.Fatal("new-message notification moved the reader's position")
			}
			resume := "end"
			if pause == " " {
				resume = " "
			}
			chatKeyRun(m, resume)
			screen = m.View()
			if strings.Contains(screen, banner) || !strings.Contains(screen, "unseen-arrival") || !m.view.AtBottom() {
				t.Fatal("returning to latest did not clear the banner and reveal the arrival")
			}
		})
	}
}

func TestChatUnreadFilterHandlesInflightToggleAndHiddenFlood(t *testing.T) {
	m := chatTestModel(t)
	runChatCommand(m, m.readChat())
	chatKeyRun(m, " ")
	for range 105 {
		if _, err := m.chat.history.Append(1, json.RawMessage(`{"cmd":"ONLINE_RANK_COUNT","data":{"count":10}}`)); err != nil {
			t.Fatal(err)
		}
	}
	runChatCommand(m, m.readChat())
	banner := i18n.T(i18n.DanmakuNewMessages)
	if strings.Contains(m.View(), banner) {
		t.Fatal("hidden rankings created a new-message notification")
	}
	if _, err := m.chat.history.Append(1, json.RawMessage(`{"cmd":"ENTRY_EFFECT","data":{"copy_writing":"optional-arrival"}}`)); err != nil {
		t.Fatal(err)
	}
	pending := m.readChat()
	queued := pending().(chatPageMsg)
	chatKeyRun(m, "f")
	runChatCommand(m, m.applyChatPage(queued))
	if !strings.Contains(m.View(), banner) || strings.Contains(m.chatView(), "optional-arrival") {
		t.Fatal("filter toggle did not re-evaluate unseen optional events while keeping empty history paused")
	}
	chatKeyRun(m, "f")
	if strings.Contains(m.View(), banner) {
		t.Fatal("hidden-only arrivals kept the banner after changing the filter")
	}
	appendChat(t, m, "first-visible-message")
	runChatCommand(m, m.readChat())
	if !strings.Contains(m.View(), banner) {
		t.Fatal("empty paused history missed its first visible arrival")
	}
	chatKeyRun(m, "end")
	if screen := m.View(); strings.Contains(screen, banner) || !strings.Contains(screen, "first-visible-message") {
		t.Fatal("empty-history pause could not return to the latest message")
	}
}

func TestChatUnreadBannerDoesNotCrossHistoryRooms(t *testing.T) {
	m := chatTestModel(t)
	appendChat(t, m, "room-one")
	if _, err := m.chat.history.Append(2, json.RawMessage(`{"cmd":"DANMU_MSG","info":[[],"room-two",[1,"viewer"]]}`)); err != nil {
		t.Fatal(err)
	}
	runChatCommand(m, m.readChat())
	chatKeyRun(m, " ")
	appendChat(t, m, "room-one-unseen")
	runChatCommand(m, m.readChat())
	banner := i18n.T(i18n.DanmakuNewMessages)
	if !strings.Contains(m.View(), banner) {
		t.Fatal("missing source-room notification")
	}
	chatKeyRun(m, "o")
	if screen := m.View(); strings.Contains(screen, banner) || !strings.Contains(screen, "room-two") {
		t.Fatal("unread notification leaked into another history room")
	}
}

func TestChatReturningToLiveRoomCannotPauseThePreviousRoom(t *testing.T) {
	m := chatTestModel(t)
	appendChat(t, m, "previous-room-message")
	if _, err := m.chat.history.Append(2, json.RawMessage(`{"cmd":"DANMU_MSG","info":[[],"live-room-message",[1,"viewer"]]}`)); err != nil {
		t.Fatal(err)
	}
	runChatCommand(m, m.readChat())
	m.chat.manualRoom = true
	m.chat.state.RoomID = 2
	_, pending := m.chatKey("end")
	if strings.Contains(m.chatView(), "previous-room-message") {
		t.Fatal("returning to live left the previous room's messages on screen")
	}
	queued := pending().(chatPageMsg)
	chatKeyRun(m, " ")
	runChatCommand(m, m.applyChatPage(queued))
	if body := m.chatView(); strings.Contains(body, "previous-room-message") || strings.Contains(body, "live-room-message") {
		t.Fatal("pausing before the live room loaded used another room's read position")
	}
	chatKeyRun(m, " ")
	if !strings.Contains(m.chatView(), "live-room-message") {
		t.Fatal("resuming did not load the live room")
	}
}

func TestChatHistoryErrorWithOtherEnabledDoesNotLoop(t *testing.T) {
	m := chatTestModel(t)
	m.chat.showOther = true
	if err := m.chat.history.Close(); err != nil {
		t.Fatal(err)
	}
	msg := m.readChat()().(chatPageMsg)
	if next := m.applyChatPage(msg); next != nil || m.chat.err == nil || m.chat.loading {
		t.Fatal("history failure must surface instead of being retried as a stale filter result")
	}
}

func TestChatShortViewportShowsMoreUntilBottom(t *testing.T) {
	m := chatTestModel(t)
	for n := range 20 {
		appendChat(t, m, fmt.Sprintf("short-message-%02d", n))
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 13})
	runChatCommand(m, m.readChat())
	hint := i18n.T(i18n.DanmakuNewMessages)
	screen := m.View()
	if !strings.Contains(screen, "short-message-19") || strings.Contains(screen, hint) {
		t.Fatal("one-line viewport must show the latest message without a false scroll hint")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyUp})
	screen = m.View()
	if !strings.Contains(screen, hint) || strings.Contains(screen, "short-message-19") {
		t.Fatal("scrolling above the latest message must reveal the scroll hint immediately")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if screen = m.View(); strings.Contains(screen, hint) || !strings.Contains(screen, "short-message-19") {
		t.Fatal("scrolling back to the bottom must clear the hint immediately")
	}
}

func TestChatScrollToUnreadKeepsFollowingPaused(t *testing.T) {
	m := chatTestModel(t)
	appendChat(t, m, "already-read")
	runChatCommand(m, m.readChat())
	m.View()
	chatKeyRun(m, " ")
	appendChat(t, m, "unread-arrival")
	runChatCommand(m, m.readChat())
	hint := i18n.T(i18n.DanmakuNewMessages)
	if !strings.Contains(m.View(), hint) {
		t.Fatal("paused arrivals must expose the scroll hint")
	}
	chatKeyRun(m, "down")
	screen := m.View()
	if !strings.Contains(screen, "unread-arrival") || strings.Contains(screen, hint) || m.chat.follow {
		t.Fatal("scrolling to unread messages must clear the hint without enabling follow")
	}
	appendChat(t, m, "next-unread")
	runChatCommand(m, m.readChat())
	screen = m.View()
	if !strings.Contains(screen, hint) || strings.Contains(screen, "next-unread") {
		t.Fatal("reading unread messages must preserve the paused history boundary")
	}
}

func TestChatListenerEventsFollowOtherFilterAndUnreadState(t *testing.T) {
	m := chatTestModel(t)
	runChatCommand(m, m.readChat())
	chatKeyRun(m, " ")
	for _, event := range []string{"session_start", "session_end", "connection_lost"} {
		if err := m.chat.history.RecordGap(1, event); err != nil {
			t.Fatal(err)
		}
	}
	runChatCommand(m, m.readChat())
	hint := i18n.T(i18n.DanmakuNewMessages)
	if strings.Contains(m.View(), hint) {
		t.Fatal("hidden listener events must not create an unread notification")
	}
	chatKeyRun(m, "f")
	if !strings.Contains(m.View(), hint) {
		t.Fatal("enabling other events must expose unread listener events")
	}
	chatKeyRun(m, "end")
	for _, key := range []i18n.Key{i18n.DanmakuSessionStart, i18n.DanmakuSessionEnd, i18n.DanmakuConnectionLost} {
		if !strings.Contains(m.chatView(), i18n.T(key)) {
			t.Fatal("enabled listener event is missing from history")
		}
	}
	chatKeyRun(m, "f")
	for _, key := range []i18n.Key{i18n.DanmakuSessionStart, i18n.DanmakuSessionEnd, i18n.DanmakuConnectionLost} {
		if strings.Contains(m.chatView(), i18n.T(key)) {
			t.Fatal("disabled listener event leaked into chat")
		}
	}
	events, err := m.chat.history.Page(1, 0, chatPageSize)
	if err != nil || len(events) != 3 {
		t.Fatalf("display filter changed saved listener history: events=%d err=%v", len(events), err)
	}
}
