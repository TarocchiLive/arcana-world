package tui

import (
	"strings"
	"time"

	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

// 可选区域只包含可见正文，不包含边框、输入框和操作控件。
type textRegion struct {
	floatingLayer
	kind string
}

type textPoint struct{ x, y int }

type textSelection struct {
	region       textRegion
	lines        []string
	anchor, head textPoint
	dragging     bool
	active       bool
}

func (r textRegion) contains(x, y int) bool {
	return r.content != "" && x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}

func (m *Model) pageTextRegion(body string) textRegion {
	kind := ""
	if m.page == chatPage && m.chat != nil && (m.mode == "" || m.mode == "chat-history") {
		kind = "chat"
	} else if m.page == logsPage && m.mode == "" {
		kind = "logs"
	}
	if kind == "" {
		return textRegion{}
	}
	return textRegion{kind: kind, floatingLayer: floatingLayer{
		content: body, x: m.pickerLeft, y: m.pickerTop, width: m.view.Width(), height: m.view.Height(),
	}}
}

func (m *Model) textRegionAt(x, y int) *textRegion {
	if m.previewing {
		return nil
	}
	// 通知优先命中，标题和边框也不能穿透到下方正文。
	if m.pointerOverNotice() {
		if r := &m.frame.textRegions[1]; r.contains(x, y) {
			return r
		}
		return nil
	}
	if r := &m.frame.textRegions[0]; r.contains(x, y) {
		return r
	}
	return nil
}

func (m *Model) selectingNotice() bool {
	return m.textSelection != nil && m.textSelection.region.kind == "notice"
}

func (m *Model) clearTextSelection() {
	if m.textSelection == nil {
		return
	}
	if m.selectingNotice() {
		m.notification.generation++
		// 用户读完选区后，重新给予完整的通知展示时间。
		m.notification.started = time.Now()
		m.notification.pending = true
	}
	m.textSelection = nil
	m.frame.view = tea.View{}
}

func (m *Model) retainTextSelection(regions [2]textRegion) {
	s := m.textSelection
	if s == nil {
		return
	}
	for _, r := range regions {
		if r.kind == s.region.kind && r.content != "" && r.x == s.region.x && r.y == s.region.y && r.width == s.region.width && r.height == s.region.height {
			return
		}
	}
	m.clearTextSelection()
}

func (s *textSelection) move(x, y int) {
	s.head = textPoint{max(0, min(x-s.region.x, s.region.width-1)), max(0, min(y-s.region.y, len(s.lines)-1))}
	if s.head != s.anchor {
		s.active = true
	}
}

// 宽字符及组合字符始终整组选择，不能从一个终端显示格中间截断。
func textCellBoundary(line string, column int, right bool) int {
	cells := 0
	for graphemes := uniseg.NewGraphemes(line); graphemes.Next(); {
		next := cells + graphemes.Width()
		if column < next {
			if right {
				return next
			}
			return cells
		}
		cells = next
	}
	return cells
}

func (s *textSelection) span(row int) (int, int) {
	if !s.active {
		return 0, 0
	}
	start, end := s.anchor, s.head
	if start.y > end.y || start.y == end.y && start.x > end.x {
		start, end = end, start
	}
	if row < start.y || row > end.y {
		return 0, 0
	}
	left, right := 0, ansi.StringWidth(s.lines[row])
	if row == start.y {
		left = textCellBoundary(s.lines[row], start.x, false)
	}
	if row == end.y {
		right = textCellBoundary(s.lines[row], end.x, true)
	}
	return left, right
}

func (s *textSelection) text() string {
	var lines []string
	for row, line := range s.lines {
		left, right := s.span(row)
		if right > left {
			lines = append(lines, strings.TrimRight(ansi.Cut(line, left, right), " \t"))
		} else if s.active && row > min(s.anchor.y, s.head.y) && row < max(s.anchor.y, s.head.y) {
			lines = append(lines, "")
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n")
}

func (m *Model) copyTextSelection() tea.Cmd {
	if m.textSelection == nil {
		return nil
	}
	text := m.textSelection.text()
	if strings.TrimSpace(text) == "" {
		return nil
	}
	return tea.SetClipboard(text)
}

// 在普通鼠标分发之前捕获拖选，松开鼠标不会触发下方按钮。
func (m *Model) textSelectionMouse(msg tea.MouseMsg) (bool, tea.Cmd) {
	mouse := msg.Mouse()
	m.mouseX, m.mouseY, m.mouseKnown = mouse.X, mouse.Y, true
	s := m.textSelection
	switch msg.(type) {
	case tea.MouseMotionMsg:
		if s == nil || !s.dragging {
			return false, nil
		}
		s.move(mouse.X, mouse.Y)
		m.frame.view = tea.View{}
		return true, nil
	case tea.MouseReleaseMsg:
		if s == nil || !s.dragging {
			return false, nil
		}
		s.move(mouse.X, mouse.Y)
		s.dragging = false
		if !s.active {
			m.clearTextSelection()
		}
		m.frame.view = tea.View{}
		return true, m.notificationCommand()
	case tea.MouseWheelMsg:
		m.clearTextSelection()
	case tea.MouseClickMsg:
		if mouse.Button != tea.MouseLeft {
			return false, nil
		}
		r := m.textRegionAt(mouse.X, mouse.Y)
		m.clearTextSelection()
		if r == nil {
			return false, nil
		}
		point := textPoint{mouse.X - r.x, mouse.Y - r.y}
		region := *r
		// 从最终画面截取正文，保留主题已经确定的逐格颜色。
		// 通知覆盖正文时使用通知下方的页面，避免把通知混入弹幕或日志。
		source := m.frame.base
		if r.kind != "notice" && m.noticeLayer.content != "" {
			source = composeLayer(m.backdrop)
		}
		rows := strings.Split(source, "\n")
		visible := make([]string, 0, r.height)
		for _, row := range rows[min(r.y, len(rows)):min(r.y+r.height, len(rows))] {
			visible = append(visible, ansi.Cut(row, r.x, r.x+r.width))
		}
		region.content = strings.Join(visible, "\n")
		m.textSelection = &textSelection{region: region, lines: strings.Split(ansi.Strip(region.content), "\n"), anchor: point, head: point, dragging: true}
		return true, nil
	}
	return false, nil
}

func (m *Model) textSelectionView(screen string) string {
	s := m.textSelection
	if s == nil {
		return screen
	}
	// 只冻结正在选择的正文，后台消息照常接收，其余界面继续更新。
	layers := make([]floatingLayer, 0, len(s.lines)+3)
	layers = append(layers, s.region.floatingLayer)
	style := lipgloss.NewStyle().Foreground(m.theme.canvasColor).Background(m.theme.accentColor)
	for row, line := range s.lines {
		left, right := s.span(row)
		if right > left {
			layers = append(layers, floatingLayer{content: style.Render(ansi.Cut(line, left, right)), x: s.region.x + left, y: s.region.y + row, width: right - left, height: 1})
		}
	}
	if s.region.kind != "notice" {
		layers = append(layers, m.noticeLayer)
	}
	if s.active {
		padding := min(2, max(0, m.width-1))
		hint := ansi.Truncate(i18n.T(i18n.LumenSelectionKeys), max(1, m.width-padding), "…")
		footer := m.theme.muted.Background(m.theme.canvasColor).PaddingLeft(padding).
			Width(m.width).MaxWidth(m.width).MaxHeight(1).Render(hint)
		layers = append(layers, floatingLayer{content: footer, y: m.height - 1, width: m.width, height: 1})
	}
	return composeLayer(screen, layers...)
}
