package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// 直接查找画面中的插入字符，不依赖布局和光标计算函数。
func requireCursorAt(t *testing.T, view tea.View, marker string) {
	t.Helper()
	for y, line := range strings.Split(ansi.Strip(view.Content), "\n") {
		if x := strings.Index(line, marker); x >= 0 {
			column := ansi.StringWidth(line[:x])
			if view.Cursor == nil || view.Cursor.X != column || view.Cursor.Y != y {
				t.Fatalf("cursor = %+v, want (%d,%d) at %q", view.Cursor, column, y, marker)
			}
			return
		}
	}
	t.Fatalf("insertion marker %q is not visible:\n%s", marker, ansi.Strip(view.Content))
}

func TestInputCursorFollowsRenderedContent(t *testing.T) {
	for _, size := range [][2]int{{120, 40}, {64, 20}, {40, 12}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := lifecycleModel(t, context.Background())
			m.dismissNotification()
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			m.form("proxy", strings.Repeat("表单说明", 12), "甲乙Z丙", false)
			m.input.SetCursor(2)
			requireCursorAt(t, m.View(), "Z")

			m.mode = "selection"
			m.selection = newTitleSelection(strings.Repeat("甲", 40)+"乙Z丙", nil)
			m.syncWorkspace()
			m.selection.input.SetCursor(41)
			requireCursorAt(t, m.View(), "Z")
			m.Update(tea.WindowSizeMsg{Width: 32, Height: 12})
			requireCursorAt(t, m.View(), "Z")
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			requireCursorAt(t, m.View(), "Z")

			m.mode = ""
			m.page = chatPage
			m.view.GotoTop()
			m.chatInput.Focus()
			m.chatInput.SetValue(strings.Repeat("甲", 20) + "乙Z丙")
			m.syncWorkspace()
			m.chatInput.SetCursorColumn(21)
			requireCursorAt(t, m.View(), "Z")
			m.Update(tea.MouseMotionMsg{X: 0, Y: 0})
			requireCursorAt(t, m.View(), "Z")
			m.Update(tea.BlurMsg{})
			if m.View().Cursor != nil {
				t.Fatal("unfocused terminal retained an input cursor")
			}
			m.Update(tea.FocusMsg{})
			requireCursorAt(t, m.View(), "Z")
			appendChat(t, m, "incoming while composing")
			runChatCommand(m, m.readChat())
			requireCursorAt(t, m.View(), "Z")
			m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
			if m.View().Cursor != nil {
				t.Fatal("confirmation retained the covered composer cursor")
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			requireCursorAt(t, m.View(), "Z")
			m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			if m.View().Cursor != nil {
				t.Fatal("leaving the composer retained an input cursor")
			}

			m.openChatHistory()
			m.chat.historyBrowser.inputs[0].SetValue("甲Z")
			m.chat.historyBrowser.inputs[0].SetCursor(1)
			requireCursorAt(t, m.View(), "Z")
			m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
			m.chat.historyBrowser.inputs[1].SetValue("乙Ω")
			m.chat.historyBrowser.inputs[1].SetCursor(1)
			requireCursorAt(t, m.View(), "Ω")
		})
	}
}

func TestInputCursorMasksAndSearch(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.dismissNotification()
	m.form("obs-password", "Password", "你好ab", true)
	m.input.SetCursor(1)
	view := m.View()
	if view.Cursor == nil {
		t.Fatal("password input has no cursor")
	}
	line := strings.Split(ansi.Strip(view.Content), "\n")[view.Cursor.Y]
	mask := strings.Index(line, "******")
	if mask < 0 || view.Cursor.X != ansi.StringWidth(line[:mask])+2 || strings.Contains(view.Content, "你好") {
		t.Fatalf("password cursor does not follow the displayed mask: %+v, %q", view.Cursor, line)
	}

	m.mode = "selection"
	m.selection = newAreaSelection(nil, nil, 0)
	m.selection.query = []rune(strings.Repeat("甲", 40) + "乙")
	m.Update(tea.WindowSizeMsg{Width: 32, Height: 12})
	view = m.View()
	if view.Cursor == nil {
		t.Fatal("category search has no cursor")
	}
	line = strings.Split(ansi.Strip(view.Content), "\n")[view.Cursor.Y]
	if prefix := ansi.Cut(line, 0, view.Cursor.X); !strings.HasSuffix(prefix, "甲乙") {
		t.Fatalf("search cursor does not follow the visible query: %+v, %q", view.Cursor, line)
	}
	m.Update(tea.WindowSizeMsg{Width: 4, Height: 3})
	if m.View().Cursor != nil {
		t.Fatal("clipped input retained an off-screen cursor")
	}
}
