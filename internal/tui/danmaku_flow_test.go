package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
)

func TestChatChronologyAndPinnedControlsFollowTheBottom(t *testing.T) {
	m := chatTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 36})
	for n := 1; n <= 100; n++ {
		appendChat(t, m, fmt.Sprintf("message-%03d", n))
	}
	runChatCommand(m, m.readChat())
	body := m.chatView(m.view.Width())
	if strings.Index(body, "message-001") >= strings.Index(body, "message-100") {
		t.Fatal("chat does not progress from older to newer")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}) // 关闭固定控件上方的启动提示。
	screen := m.View().Content
	if !m.view.AtBottom() || !strings.Contains(screen, "message-100") {
		t.Fatal("following did not reveal the newest message at the bottom")
	}
	if strings.Contains(body, i18n.T(i18n.DanmakuOther)) || !strings.Contains(screen, i18n.T(i18n.DanmakuOther)) {
		t.Fatal("Other control must stay outside the scrolling message body")
	}
	appendChat(t, m, "newest-live-arrival")
	runChatCommand(m, m.readChat())
	if screen := m.View().Content; !m.view.AtBottom() || !strings.Contains(screen, "newest-live-arrival") {
		t.Fatal("live arrival did not advance the bottom")
	}
	m.Update(tea.WindowSizeMsg{Width: 64, Height: 20})
	if screen := m.View().Content; !m.view.AtBottom() || !strings.Contains(screen, "newest-live-arrival") {
		t.Fatal("switching to compact layout lost the live-follow position")
	}
}

func TestChatPausedAndPagedReadersSeeNewMessageBanner(t *testing.T) {
	for _, pause := range []string{"space", "["} {
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
			offset, frozen := m.view.YOffset(), m.chatView(m.view.Width())
			banner := i18n.T(i18n.DanmakuNewMessages)
			if !strings.Contains(m.View().Content, banner) {
				t.Fatal("messages below the viewport need a scroll hint")
			}
			appendChat(t, m, "unseen-arrival")
			runChatCommand(m, m.readChat())
			screen := m.View().Content
			if !strings.Contains(screen, banner) || strings.Contains(screen, "unseen-arrival") {
				t.Fatal("paused reader needs a banner, not a replaced history page")
			}
			if m.chatView(m.view.Width()) != frozen || m.view.YOffset() != offset {
				t.Fatal("new-message notification moved the reader's position")
			}
			resume := "end"
			if pause == "space" {
				resume = "space"
			}
			chatKeyRun(m, resume)
			screen = m.View().Content
			if strings.Contains(screen, banner) || !strings.Contains(screen, "unseen-arrival") || !m.view.AtBottom() {
				t.Fatal("returning to latest did not clear the banner and reveal the arrival")
			}
		})
	}
}

func TestChatUnreadFilterHandlesInflightToggleAndHiddenFlood(t *testing.T) {
	m := chatTestModel(t)
	runChatCommand(m, m.readChat())
	chatKeyRun(m, "space")
	for range 105 {
		if _, err := m.chat.history.Append(1, json.RawMessage(`{"cmd":"ONLINE_RANK_COUNT","data":{"count":10}}`)); err != nil {
			t.Fatal(err)
		}
	}
	runChatCommand(m, m.readChat())
	banner := i18n.T(i18n.DanmakuNewMessages)
	if strings.Contains(m.View().Content, banner) {
		t.Fatal("hidden rankings created a new-message notification")
	}
	if _, err := m.chat.history.Append(1, json.RawMessage(`{"cmd":"ENTRY_EFFECT","data":{"copy_writing":"optional-arrival"}}`)); err != nil {
		t.Fatal(err)
	}
	pending := m.readChat()
	queued := pending().(chatPageMsg)
	chatKeyRun(m, "f")
	runChatCommand(m, m.applyChatPage(queued))
	if !strings.Contains(m.View().Content, banner) || strings.Contains(m.chatView(m.view.Width()), "optional-arrival") {
		t.Fatal("filter toggle did not re-evaluate unseen optional events while keeping empty history paused")
	}
	chatKeyRun(m, "f")
	if strings.Contains(m.View().Content, banner) {
		t.Fatal("hidden-only arrivals kept the banner after changing the filter")
	}
	appendChat(t, m, "first-visible-message")
	runChatCommand(m, m.readChat())
	if !strings.Contains(m.View().Content, banner) {
		t.Fatal("empty paused history missed its first visible arrival")
	}
	chatKeyRun(m, "end")
	if screen := m.View().Content; strings.Contains(screen, banner) || !strings.Contains(screen, "first-visible-message") {
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
	chatKeyRun(m, "space")
	appendChat(t, m, "room-one-unseen")
	runChatCommand(m, m.readChat())
	banner := i18n.T(i18n.DanmakuNewMessages)
	if !strings.Contains(m.View().Content, banner) {
		t.Fatal("missing source-room notification")
	}
	chatKeyRun(m, "o")
	if screen := m.View().Content; strings.Contains(screen, banner) || !strings.Contains(screen, "room-two") {
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
	if strings.Contains(m.chatView(m.view.Width()), "previous-room-message") {
		t.Fatal("returning to live left the previous room's messages on screen")
	}
	queued := pending().(chatPageMsg)
	chatKeyRun(m, "space")
	runChatCommand(m, m.applyChatPage(queued))
	if body := m.chatView(m.view.Width()); strings.Contains(body, "previous-room-message") || strings.Contains(body, "live-room-message") {
		t.Fatal("pausing before the live room loaded used another room's read position")
	}
	chatKeyRun(m, "space")
	if !strings.Contains(m.chatView(m.view.Width()), "live-room-message") {
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
	screen := m.View().Content
	if !strings.Contains(screen, "short-message-19") || strings.Contains(screen, hint) {
		t.Fatal("one-line viewport must show the latest message without a false scroll hint")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	screen = m.View().Content
	if !strings.Contains(screen, hint) || strings.Contains(screen, "short-message-19") {
		t.Fatal("scrolling above the latest message must reveal the scroll hint immediately")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if screen = m.View().Content; strings.Contains(screen, hint) || !strings.Contains(screen, "short-message-19") {
		t.Fatal("scrolling back to the bottom must clear the hint immediately")
	}
}

func TestChatScrollToUnreadKeepsFollowingPaused(t *testing.T) {
	m := chatTestModel(t)
	appendChat(t, m, "already-read")
	runChatCommand(m, m.readChat())
	m.View()
	chatKeyRun(m, "space")
	appendChat(t, m, "unread-arrival")
	runChatCommand(m, m.readChat())
	hint := i18n.T(i18n.DanmakuNewMessages)
	if !strings.Contains(m.View().Content, hint) {
		t.Fatal("paused arrivals must expose the scroll hint")
	}
	chatKeyRun(m, "down")
	screen := m.View().Content
	if !strings.Contains(screen, "unread-arrival") || strings.Contains(screen, hint) || m.chat.follow {
		t.Fatal("scrolling to unread messages must clear the hint without enabling follow")
	}
	appendChat(t, m, "next-unread")
	runChatCommand(m, m.readChat())
	screen = m.View().Content
	if !strings.Contains(screen, hint) || strings.Contains(screen, "next-unread") {
		t.Fatal("reading unread messages must preserve the paused history boundary")
	}
}

func TestChatListenerEventsFollowOtherFilterAndUnreadState(t *testing.T) {
	m := chatTestModel(t)
	runChatCommand(m, m.readChat())
	chatKeyRun(m, "space")
	for _, event := range []string{"session_start", "session_end", "connection_lost"} {
		if err := m.chat.history.RecordGap(1, event); err != nil {
			t.Fatal(err)
		}
	}
	runChatCommand(m, m.readChat())
	hint := i18n.T(i18n.DanmakuNewMessages)
	if strings.Contains(m.View().Content, hint) {
		t.Fatal("hidden listener events must not create an unread notification")
	}
	chatKeyRun(m, "f")
	if !strings.Contains(m.View().Content, hint) {
		t.Fatal("enabling other events must expose unread listener events")
	}
	chatKeyRun(m, "end")
	for _, key := range []i18n.Key{i18n.DanmakuSessionStart, i18n.DanmakuSessionEnd, i18n.DanmakuConnectionLost} {
		if !strings.Contains(m.chatView(m.view.Width()), i18n.T(key)) {
			t.Fatal("enabled listener event is missing from history")
		}
	}
	chatKeyRun(m, "f")
	for _, key := range []i18n.Key{i18n.DanmakuSessionStart, i18n.DanmakuSessionEnd, i18n.DanmakuConnectionLost} {
		if strings.Contains(m.chatView(m.view.Width()), i18n.T(key)) {
			t.Fatal("disabled listener event leaked into chat")
		}
	}
	events, err := m.chat.history.Page(1, 0, chatPageSize)
	if err != nil || len(events) != 3 {
		t.Fatalf("display filter changed saved listener history: events=%d err=%v", len(events), err)
	}
}

func TestChatHistoryMouseActionsAcrossLayouts(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 140, Height: 36}, {Width: 60, Height: 13}} {
		t.Run(fmt.Sprintf("%dx%d", size.Width, size.Height), func(t *testing.T) {
			m := chatTestModel(t)
			m.Update(size)
			m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			for n := 1; n <= 205; n++ {
				appendChat(t, m, fmt.Sprintf("record-%03d", n))
			}
			runChatCommand(m, m.readChat())
			click := func(target mouseTarget) {
				runChatCommand(m, m.mouse(tea.MouseClickMsg{X: target.x, Y: target.y, Button: tea.MouseLeft}))
			}
			action := func(key string) {
				t.Helper()
				m.View()
				for _, target := range m.mouseTargets {
					if target.kind == "chat" && target.key.String() == key {
						click(target)
						return
					}
				}
				for _, target := range m.mouseTargets {
					if target.kind == "chat" && target.key.String() == "a" {
						click(target)
						break
					}
				}
				if m.mode != "pick" {
					t.Fatalf("missing accessible chat action %q", key)
				}
				for index, ch := range m.choices {
					if ch.value != key {
						continue
					}
					m.selected = index
					m.View()
					for _, target := range m.mouseTargets {
						if target.kind == "pick" && target.index == index {
							click(target)
							return
						}
					}
				}
				t.Fatalf("chat action %q is not clickable", key)
			}
			action("[")
			if body := m.chatView(m.view.Width()); !strings.Contains(body, "record-105") || strings.Contains(body, "record-205") || m.chat.follow {
				t.Fatal("older action did not open paused historical records")
			}
			action("]")
			if !strings.Contains(m.chatView(m.view.Width()), "record-205") {
				t.Fatal("newer action did not restore the newer history page")
			}
			action("[")
			appendChat(t, m, "latest-arrival")
			runChatCommand(m, m.readChat())
			action("end")
			if screen := m.View().Content; !m.chat.follow || !m.view.AtBottom() || !strings.Contains(screen, "latest-arrival") {
				t.Fatal("latest action did not resume following at the newest message")
			}
		})
	}
}
