package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

type pointerRefreshMsg struct{}

// 命令结果在本次 View 后处理，此时点击区域和通知遮挡已经重建。
func refreshPointer() tea.Msg { return pointerRefreshMsg{} }

func (m *Model) pointerOverNotice() bool {
	notice := m.noticeLayer
	return m.mouseKnown && m.notificationVisible() && notice.content != "" &&
		m.mouseX >= notice.x && m.mouseX < notice.x+notice.width && m.mouseY >= notice.y && m.mouseY < notice.y+notice.height
}

// 悬浮与点击共用命中区域；遮挡、尺寸变化或模式变化后不复用旧目标。
func (m *Model) hoveredTarget() *mouseTarget {
	if !m.mouseKnown || m.previewing || m.pointerOverNotice() {
		return nil
	}
	for i := range m.mouseTargets {
		target := &m.mouseTargets[i]
		if target.page == m.page && target.mode == m.mode && target.editKind == m.editKind &&
			target.screenWidth == m.width && target.screenHeight == m.height &&
			m.mouseY == target.y && m.mouseX >= target.x && m.mouseX < target.x+target.width {
			return target
		}
	}
	return nil
}

// 只高亮鼠标所在的未遮挡区间，避免重涂位于同一行的通知。
func (m *Model) hoverBounds(target *mouseTarget) (x, width int) {
	if target == nil {
		return 0, 0
	}
	x, end := target.x, target.x+target.width
	notice := m.noticeLayer
	if notice.content != "" && m.notificationVisible() && target.y >= notice.y && target.y < notice.y+notice.height {
		switch {
		case m.mouseX < notice.x:
			end = min(end, notice.x)
		case m.mouseX >= notice.x+notice.width:
			x = max(x, notice.x+notice.width)
		default:
			return 0, 0
		}
	}
	return x, max(0, end-x)
}

// OSC 22 控制鼠标指针，不改变输入光标；不支持该协议的终端会忽略它。
func (m *Model) pointerCommand() tea.Cmd {
	if !m.initialized {
		return nil
	}
	shape := "default"
	if !m.previewing {
		if target := m.hoveredTarget(); target != nil {
			shape = "pointer"
			if target.kind == "input" || target.kind == "title" || target.kind == "chat-input" || target.kind == "history-input" {
				shape = "text"
			}
		} else if m.pointerOverNotice() && m.mouseX == m.noticeLayer.x+m.noticeLayer.width-3 && m.mouseY == m.noticeLayer.y+1 {
			shape = "pointer"
		}
	}
	if shape == m.pointerShape {
		return nil
	}
	m.pointerShape = shape
	return tea.Raw(ansi.SetPointerShape(shape))
}

// 高亮仅覆盖最终可见的一行，不移动键盘选中项，也不改写冻结的弹窗背景。
func (m *Model) hoverLayer(screen string) floatingLayer {
	if m.previewing {
		return floatingLayer{}
	}
	if m.pointerOverNotice() {
		x, y := m.noticeLayer.x+m.noticeLayer.width-3, m.noticeLayer.y+1
		if m.mouseX == x && m.mouseY == y {
			return floatingLayer{content: m.theme.hoverStyle.Render("×"), x: x, y: y, width: 1, height: 1}
		}
		return floatingLayer{}
	}
	target := m.hoveredTarget()
	if target == nil || target.kind == "input" || target.kind == "title" || target.kind == "chat-input" || target.kind == "history-input" {
		return floatingLayer{}
	}
	x, width := m.hoverBounds(target)
	if width == 0 {
		return floatingLayer{}
	}
	line := screen
	for range target.y {
		_, rest, ok := strings.Cut(line, "\n")
		if !ok {
			return floatingLayer{}
		}
		line = rest
	}
	line, _, _ = strings.Cut(line, "\n")
	text := ansi.Strip(ansi.Cut(line, x, x+width))
	style := m.theme.hoverStyle
	if target.kind == "confirm" && target.index == 1 {
		style = style.Foreground(m.theme.danger.GetForeground()).Bold(true)
	}
	return floatingLayer{content: style.Width(width).MaxWidth(width).Render(text), x: x, y: target.y, width: width, height: 1}
}
