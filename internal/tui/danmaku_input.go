package tui

import (
	"context"
	"strings"
	"unicode/utf8"

	"arcana-world/internal/i18n"
	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

const chatInputLimit = 40

func (m *Model) chatInputActive() bool {
	return m.page == chatPage && m.mode == "" && m.chatInput.Focused()
}

func (m *Model) chatComposerView(l workspaceLayout) string {
	view := m.chatInput.View()
	// View 会刷新换行后的内容；缩放时，先前的 SetHeight 可能仍按旧行数滚动。
	if c := m.chatInput.Cursor(); c != nil && (c.Y < 0 || c.Y >= l.chatComposer) {
		m.chatInput.SetHeight(l.chatComposer)
		view = m.chatInput.View()
	}
	return view
}

func (m *Model) updateChatInput(msg tea.Msg) tea.Cmd {
	atBottom := m.view.AtBottom()
	// 将剩余字符额度换算为组件使用的显示格数限制。
	selected := m.chatInput.SelectedText()
	available := chatInputLimit - utf8.RuneCountInString(m.chatInput.Value()) + utf8.RuneCountInString(selected)
	cells := m.chatInput.Length() - strings.Count(selected, "\n")
	for _, line := range strings.Split(selected, "\n") {
		cells -= uniseg.StringWidth(line)
	}
	m.chatInput.CharLimit = max(1, cells+available)
	var cmd tea.Cmd
	m.chatInput, cmd = m.chatInput.Update(msg)
	if value := m.chatInput.Value(); utf8.RuneCountInString(value) > chatInputLimit {
		m.chatInput.SetValue(string([]rune(value)[:chatInputLimit]))
	}
	if atBottom && m.chat != nil {
		m.chat.scrollToLatest = true
	}
	return cmd
}

func (m *Model) styleChatInput() {
	styles := textarea.DefaultStyles(m.theme.dark)
	styles.Focused = textarea.StyleState{
		Base: m.theme.textStyle, Text: m.theme.textStyle, Prompt: m.theme.accent,
		Placeholder: m.theme.muted, EndOfBuffer: m.theme.muted,
	}
	styles.Blurred = styles.Focused
	styles.Blurred.Prompt = m.theme.muted
	styles.Cursor.Color = m.theme.accentColor
	m.chatInput.SetStyles(styles)
	m.chatInput.Placeholder = i18n.T(i18n.DanmakuInputPlaceholder)
}

// 按输入框自动换行后的光标位置匹配鼠标坐标。
func (m *Model) focusChatInput(column, row int) tea.Cmd {
	cmd := m.chatInput.Focus()
	row += m.chatInput.ScrollYOffset()
	m.chatInput.MoveToBegin()
	for range row {
		m.chatInput.CursorDown()
	}
	info := m.chatInput.LineInfo()
	m.chatInput.SetCursorColumn(info.StartColumn)
	value := []rune(strings.Split(m.chatInput.Value(), "\n")[m.chatInput.Line()])
	for m.chatInput.Column() < min(len(value), info.StartColumn+info.CharWidth) {
		if ansi.StringWidth(string(value[info.StartColumn:m.chatInput.Column()+1])) > max(0, column-ansi.StringWidth(m.chatInput.Prompt)) {
			break
		}
		m.chatInput.SetCursorColumn(m.chatInput.Column() + 1)
	}
	return cmd
}

func (m *Model) sendChat() tea.Cmd {
	if m.busy {
		return nil
	}
	text := strings.TrimSpace(m.chatInput.Value())
	if text == "" {
		return nil
	}
	if utf8.RuneCountInString(text) > chatInputLimit {
		m.warn(i18n.T(i18n.DanmakuInputTooLong))
		return nil
	}
	if m.account == nil || m.room == nil || m.room.ID <= 0 {
		m.warn(i18n.T(i18n.DanmakuSendNoRoom))
		return nil
	}
	roomID, client := m.room.ID, m.client
	op := operation[struct{}]{
		label: i18n.DanmakuSend,
		handle: func(m *Model, _ struct{}, err error, label i18n.Key) tea.Cmd {
			if err == nil {
				m.chatInput.Reset()
			}
			return m.completeOperation(label, err)
		},
	}
	return work(m, op, func(ctx context.Context) (struct{}, error) {
		return struct{}{}, client.SendDanmaku(ctx, roomID, text)
	})
}
