package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"arcana-world/internal/i18n"
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
			if strings.Contains(m.View(), banner) {
				t.Fatal("already-read newer history was mistaken for new arrivals")
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
