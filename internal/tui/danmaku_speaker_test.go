package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/domain"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestSpeakerClickPreservesTextDragging(t *testing.T) {
	for _, width := range []int{120, 36} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			m := lifecycleModel(t, context.Background())
			m.config.ActiveUID = "1"
			m.config.ExitLiveStopDisabled, m.config.ExitOBSStopDisabled = true, true
			if err := m.store.SaveConfig(m.config); err != nil {
				t.Fatal(err)
			}
			m.account = &domain.Account{UID: "1"}
			m.room = &domain.Room{ID: 2}
			m.page = chatPage
			m.chat.loaded = true
			m.chat.entries = []danmaku.Event{{RoomID: 2, Kind: "chat", UID: "7", User: "宽名组合é以及很长的说话人名字", Text: "正文可以选择", Time: time.Now()}}
			m.chatInput.SetValue("draft")
			m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
			point := func(marker string) (int, int) {
				t.Helper()
				screen := ansi.Strip(m.View().Content)
				for y, line := range strings.Split(screen, "\n") {
					if x := strings.Index(line, marker); x >= 0 {
						return ansi.StringWidth(line[:x]), y
					}
				}
				t.Fatalf("missing %q in %s", marker, screen)
				return 0, 0
			}
			x, y := point("宽名")
			m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
			if m.speaker != nil {
				t.Fatal("press opened profile before drag decision")
			}
			m.Update(tea.MouseMotionMsg{X: x + 3, Y: y, Button: tea.MouseLeft})
			m.Update(tea.MouseReleaseMsg{X: x + 3, Y: y, Button: tea.MouseLeft})
			if m.speaker != nil || m.textSelection == nil || m.textSelection.text() != "宽名" {
				t.Fatal("name drag failed to select wide characters")
			}
			m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
			m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
			if m.speaker != nil {
				t.Fatal("clearing a frozen selection opened a profile")
			}
			x, y = point("正文")
			m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
			m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
			if m.speaker != nil {
				t.Fatal("message body opened a speaker popup")
			}
			x, y = point("宽名")
			m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
			m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
			if m.speaker == nil || !strings.Contains(ansi.Strip(m.View().Content), "UID 7") {
				t.Fatal("name click failed to show speaker identity")
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			if m.speaker != nil || m.chatInput.Value() != "draft" {
				t.Fatal("close lost chat draft or retained popup")
			}
		})
	}
}

func TestHistoricalSpeakerClickSurvivesLiveArrivalAndReturn(t *testing.T) {
	m := chatTestModel(t)
	m.config.ActiveUID = "1"
	m.account = &domain.Account{UID: "1"}
	m.room = &domain.Room{ID: 1}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	appendSpeaker := func(uid int, name, text string) {
		t.Helper()
		raw := json.RawMessage(fmt.Sprintf(`{"cmd":"DANMU_MSG","info":[[],%q,[%d,%q]]}`, text, uid, name))
		if _, err := m.chat.history.Append(1, raw); err != nil {
			t.Fatal(err)
		}
	}
	for n := range 40 {
		appendSpeaker(7, "old-speaker", fmt.Sprintf("old-record-%02d", n))
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
	m.View()
	m.view.SetYOffset(3)
	offset := m.view.YOffset()
	for range 35 {
		appendSpeaker(99, "new-speaker", "live-only")
	}
	runChatCommand(m, m.readChat())
	clickOldSpeaker := func() {
		t.Helper()
		screen := ansi.Strip(m.View().Content)
		if strings.Contains(screen, "live-only") || m.view.YOffset() != offset {
			t.Fatal("live arrival or modal return replaced the historical viewport")
		}
		for y, line := range strings.Split(screen, "\n") {
			if x := strings.Index(line, "old-speaker"); x >= 0 {
				x = ansi.StringWidth(line[:x])
				m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
				m.Update(tea.MouseReleaseMsg{X: x, Y: y, Button: tea.MouseLeft})
				if m.speaker == nil || m.speaker.uid != 7 {
					t.Fatal("visible historical name targeted a different speaker")
				}
				m.View()
				m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
				return
			}
		}
		t.Fatal("historical speaker missing from viewport")
	}
	clickOldSpeaker()
	clickOldSpeaker()
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m.View()
	runChatCommand(m, m.readChatHistory())
	if screen := ansi.Strip(m.View().Content); !strings.Contains(screen, "old-record-00") {
		t.Fatal("same-range query retained the range form or modal content")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
	if screen := ansi.Strip(m.View().Content); !strings.Contains(screen, "live-only") {
		t.Fatal("requery did not include the new record at the end")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m.View()
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m.View()
	if m.mode != "" || m.view.YOffset() != 0 || m.chatInput.Value() != "keep draft" || !strings.Contains(m.chatView(100), "live-only") {
		t.Fatalf("history exit: mode=%q offset=%d draft=%q live=%t", m.mode, m.view.YOffset(), m.chatInput.Value(), strings.Contains(m.chatView(100), "live-only"))
	}
}
