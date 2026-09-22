package tui

import tea "charm.land/bubbletea/v2"

// 缓存正文和最终画面，鼠标、空轮询与装饰更新无需重新排版页面。
type renderFrame struct {
	base               string
	view               tea.View
	cursor             *tea.Cursor
	textRegions        [2]textRegion
	target             *mouseTarget
	hoverX, hoverWidth int
	noticeClose        bool
	reuse              bool
	poll               pollViewState
	separator          floatingLayer
	separatorDirty     bool
}

// 轮询只比较可见的动态状态，不为检查变化而重新排版正文。
type pollViewState struct {
	notification uint64
	chatRoom     int64
	chatRevision uint64
	chatPhase    string
	chatError    string
	chatLoading  bool
	output       string
}

func (m *Model) pollViewState() pollViewState {
	state := pollViewState{notification: m.notification.generation}
	if m.mode != "" {
		return state
	}
	switch m.page {
	case chatPage:
		if c := m.chat; c != nil {
			state.chatRoom, state.chatRevision = c.state.RoomID, c.state.Revision
			state.chatPhase, state.chatLoading = string(c.state.Phase), c.loading
			if c.state.Err != nil {
				state.chatError = c.state.Err.Error()
			}
		}
	case ttsPage:
		state.output = m.ttsStateText()
	case overlayPage:
		state.output = m.overlayStateText()
	}
	return state
}

func (m *Model) frameView() tea.View {
	frame := &m.frame
	cursor := frame.cursor
	if m.blurred || m.textSelection != nil {
		cursor = nil
	}
	target := m.hoveredTarget()
	if target != nil && (target.kind == "input" || target.kind == "title" || target.kind == "chat-input" || target.kind == "history-input") {
		target = nil
	}
	x, width := m.hoverBounds(target)
	close := !m.previewing && m.pointerOverNotice() &&
		m.mouseX == m.noticeLayer.x+m.noticeLayer.width-3 && m.mouseY == m.noticeLayer.y+1
	if frame.view.Content != "" && frame.view.Cursor == cursor && frame.target == target && frame.noticeClose == close && frame.hoverX == x && frame.hoverWidth == width {
		return frame.view
	}
	screen := m.textSelectionView(frame.base)
	if layer := m.hoverLayer(screen); layer.content != "" {
		screen = composeRow(screen, layer)
	}
	view := tea.NewView(screen)
	view.AltScreen = true
	view.MouseMode = tea.MouseModeAllMotion
	view.ReportFocus = true
	view.Cursor = cursor
	frame.view, frame.target, frame.noticeClose = view, target, close
	frame.hoverX, frame.hoverWidth = x, width
	return view
}
