package tui

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Bubbles 2.2.1 的 SetWidth 不会同步刷新单行输入框的可见范围。
func resizeTextInput(input *textinput.Model, width int) {
	if input.Width() == width {
		return
	}
	position := input.Position()
	input.SetWidth(width)
	input.CursorEnd()
	input.SetCursor(position)
}

// Bubbles 2.2.1 的单行光标使用字符序号，未计入横向滚动偏移。
// 与鼠标定位共用渲染前缀测量，兼顾宽字符、密码掩码和隐藏输入。
func textInputCursorColumn(input textinput.Model) (int, bool) {
	styles := input.Styles()
	if input.Value() == "" {
		return ansi.StringWidth(styles.Focused.Prompt.Render(input.Prompt)), true
	}
	styles.Focused.Text = lipgloss.NewStyle().Transform(func(value string) string { return value + "\x00" })
	input.SetStyles(styles)
	rendered := input.View()
	marker := strings.IndexByte(rendered, 0)
	if marker < 0 {
		return 0, false
	}
	return ansi.StringWidth(rendered[:marker]), true
}

func textInputCursor(input textinput.Model) *tea.Cursor {
	c := input.Cursor()
	if c == nil {
		return nil
	}
	x, ok := textInputCursorColumn(input)
	if !ok {
		return nil
	}
	c.X = x
	return c
}

// 返回页面内容中的光标坐标，尚未扣除视口滚动偏移。
func (m *Model) contentCursor() *tea.Cursor {
	if m.busy || m.previewing || m.exitPrompt != nil {
		return nil
	}
	var c *tea.Cursor
	switch m.mode {
	case "form":
		c = textInputCursor(m.input)
		if c != nil {
			c.Y += lipgloss.Height(m.formPrompt())
		}
	case "selection":
		if m.selection != nil {
			c = m.selection.cursor(m.view.Width(), m.view.Height())
		}
	case "chat-history-range":
		if m.chat != nil && m.chat.historyBrowser != nil {
			b := m.chat.historyBrowser
			c = textInputCursor(b.inputs[b.focus])
			if c != nil {
				c.Y += 3 + 3*b.focus
			}
		}
	}
	return c
}

func (m *Model) screenCursor(l workspaceLayout, c *tea.Cursor) *tea.Cursor {
	if m.chatInputActive() && !m.busy && !m.previewing && m.exitPrompt == nil && l.chatComposer > 0 {
		c = m.chatInput.Cursor()
		if c == nil || c.X < 0 || c.X >= l.innerWidth-2*l.chatBorder || c.Y < 0 || c.Y >= l.chatComposer {
			return nil
		}
		c.Y += m.view.Height() + l.chatPosition + l.chatDivider
	} else {
		if c == nil {
			return nil
		}
		c.Y -= m.view.YOffset()
		if c.X < 0 || c.X >= m.view.Width() || c.Y < 0 || c.Y >= m.view.Height() {
			return nil
		}
	}
	c.X += m.pickerLeft
	c.Y += m.pickerTop
	if c.X >= m.width || c.Y >= m.height {
		return nil
	}
	// 通知遮挡输入区域时，不让光标穿透通知内容。
	n := m.noticeLayer
	if n.content != "" && c.X >= n.x && c.X < n.x+n.width && c.Y >= n.y && c.Y < n.y+n.height {
		return nil
	}
	return c
}
