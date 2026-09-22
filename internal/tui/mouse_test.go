package tui

import (
	"context"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestMouseInputInsertionUsesVisibleCells(t *testing.T) {
	for _, tc := range []struct {
		name, value, want string
		width, column     int
		masked            bool
	}{
		{"wide characters", "你好ab", "你X好ab", 20, 4, false},
		{"masked wide characters", "你好ab", "你X好ab", 20, 4, true},
		{"scrolled repeated text", strings.Repeat("x", 30) + "末尾", strings.Repeat("x", 30) + "末X尾", 8, -1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			input := textinput.New()
			input.SetWidth(tc.width)
			if tc.masked {
				input.EchoMode = textinput.EchoPassword
			}
			input.SetValue(tc.value)
			input.CursorEnd()
			column := tc.column
			if column < 0 {
				column = ansi.StringWidth(strings.TrimRight(ansi.Strip(input.View()), " ")) - 2
			}
			mouseFocusInput(&input, column)
			input, _ = input.Update(tea.KeyPressMsg{Code: 'X', Text: "X"})
			if got := input.Value(); got != tc.want {
				t.Fatalf("click at cell %d inserted into %q, want %q", column, got, tc.want)
			}
			if tc.masked && strings.Contains(ansi.Strip(input.View()), "你好") {
				t.Fatal("clicking a masked input exposed its text")
			}
		})
	}
}

func TestMouseHoverDoesNotChangeKeyboardAction(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.page = settingsPage
	for index, item := range m.menu() {
		if item.action == "theme" {
			m.cursors[settingsPage] = index
			break
		}
	}
	m.dismissNotification()
	m.Init()
	before := m.View().Content
	var hover mouseTarget
	found := false
	for _, target := range m.mouseTargets {
		if target.kind == "action" && m.menu()[target.index].action == "proxy" {
			hover, found = target, true
			break
		}
	}
	if !found {
		t.Fatal("proxy action is not visible")
	}
	_, pointer := m.Update(tea.MouseMotionMsg{X: hover.x, Y: hover.y})
	highlighted := m.View().Content
	if before == highlighted || ansi.Strip(before) != ansi.Strip(highlighted) {
		t.Fatal("hover should change color without changing visible text")
	}
	if pointer == nil {
		t.Fatal("hover did not request an interactive pointer")
	}
	if _, next := m.Update(pointer()); next != nil {
		t.Fatal("writing the pointer shape triggered a feedback command")
	}
	if got := m.View().Content; got != highlighted {
		t.Fatal("writing the pointer shape changed the hovered screen")
	}
	// 同一选项内移动不改变高亮；只重绘一行时也不能污染相邻行样式。
	m.Update(tea.MouseMotionMsg{X: hover.x + hover.width - 1, Y: hover.y})
	if got := m.View().Content; got != highlighted {
		t.Fatal("moving within one action changed its highlight")
	}
	normalize := func(screen string) string {
		width, height := lipgloss.Size(screen)
		return lipgloss.NewCanvas(width, height).Compose(lipgloss.NewCompositor(lipgloss.NewLayer(screen))).Render()
	}
	if want := composeLayer(before, m.hoverLayer(before)); normalize(highlighted) != normalize(want) {
		t.Fatal("hover changed cells outside its visible action")
	}
	m.Update(tea.MouseMotionMsg{X: 0, Y: 0})
	if restored := m.View().Content; restored != before {
		t.Fatal("leaving the action retained its hover style")
	}
	m.Update(tea.MouseMotionMsg{X: hover.x, Y: hover.y})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.mode != "pick" || m.editKind != "theme" {
		t.Fatal("hover changed the keyboard-selected action")
	}
	modal := m.View().Content
	if ansi.Strip(modal) == ansi.Strip(before) {
		t.Fatal("keyboard action did not replace the hovered page")
	}
	m.Update(tea.MouseMotionMsg{X: hover.x, Y: hover.y})
	m.Update(tea.WindowSizeMsg{Width: 64, Height: 20})
	// 窗口变化与随后收到的移动之间可能还没有绘制，不能复用旧尺寸的帧。
	m.Update(tea.MouseMotionMsg{X: 0, Y: 0})
	resized := m.View().Content
	if width, height := lipgloss.Size(resized); width != 64 || height != 20 {
		t.Fatalf("hover retained stale viewport dimensions: %dx%d", width, height)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	closed := m.View().Content
	m.Update(tea.MouseMotionMsg{X: 0, Y: 0})
	if restored := m.View().Content; restored != closed {
		t.Fatal("hover retained the closed picker")
	}
}

func TestHoverDoesNotRepaintCoveringNotification(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.page = settingsPage
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 32})
	m.warnStatus(strings.Repeat("notification detail\n", 12))
	before := m.View().Content
	card := m.noticeLayer
	for _, target := range m.mouseTargets {
		if target.kind != "action" || target.x >= card.x || target.y < card.y || target.y >= card.y+card.height {
			continue
		}
		m.Update(tea.MouseMotionMsg{X: target.x, Y: target.y})
		after := m.View().Content
		if before == after {
			t.Fatal("uncovered action did not highlight")
		}
		region := func(screen string) string {
			row := strings.Split(screen, "\n")[target.y]
			return composeLayer(ansi.Cut(row, card.x, card.x+card.width))
		}
		if region(before) != region(after) {
			t.Fatal("hovering the uncovered part of an action repainted the notification")
		}
		return
	}
	t.Fatal("no partially covered action found")
}

func TestStationaryPointerSurvivesRedraw(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.page = settingsPage
	m.config.TUIMotionDisabled = true
	m.dismissNotification()
	m.Init()
	m.View()
	var action *mouseTarget
	for i := range m.mouseTargets {
		if m.mouseTargets[i].kind == "action" {
			action = &m.mouseTargets[i]
			break
		}
	}
	if action == nil {
		t.Fatal("no visible action")
	}
	_, hover := m.Update(tea.MouseMotionMsg{X: action.x, Y: action.y})
	m.View()
	if hover == nil {
		t.Fatal("hover did not request a pointer")
	}
	initial := hover()
	if raw, ok := initial.(tea.RawMsg); !ok || raw.Msg != ansi.SetPointerShape("pointer") {
		t.Fatal("action did not show an interactive pointer")
	}
	m.Update(initial)
	m.View()
	_, cmd := m.Update(notificationExpired{})
	m.View()
	pending := []tea.Cmd{cmd}
	for len(pending) != 0 {
		next := pending[0]
		pending = pending[1:]
		if next == nil {
			continue
		}
		msg := next()
		if batch, ok := msg.(tea.BatchMsg); ok {
			pending = append(pending, batch...)
			continue
		}
		if raw, ok := msg.(tea.RawMsg); ok && raw.Msg != ansi.SetPointerShape("pointer") {
			t.Fatal("redraw reset the stationary pointer over an action")
		}
		_, followup := m.Update(msg)
		m.View()
		pending = append(pending, followup)
	}
}
