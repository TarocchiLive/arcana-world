package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"arcana-world/internal/i18n"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/rivo/uniseg"
)

type mouseTarget struct {
	x, y, width                     int
	kind                            string
	index                           int
	key                             tea.KeyPressMsg
	page, screenWidth, screenHeight int
	mode, editKind                  string
}

type mouseControl struct {
	label i18n.Key
	key   tea.KeyPressMsg
}

func mouseRuneKey(value rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: value, Text: string(value)}
}

func (m *Model) mouseControlItems() []mouseControl {
	back := mouseControl{i18n.LumenMouseBack, tea.KeyPressMsg{Code: tea.KeyEsc}}
	if m.exitPrompt != nil {
		return nil
	}
	if m.previewing || (m.page == chatPage && (m.mode == "" || m.mode == "chat-history")) {
		return nil
	}
	if m.busy {
		if m.mode == "speaker" {
			return nil
		}
		if m.cancel != nil {
			return []mouseControl{back}
		}
		return nil
	}
	if m.obsBusy && m.mode == "" && m.obsCancel != nil {
		return []mouseControl{back}
	}
	submit := mouseControl{i18n.LumenMouseSubmit, tea.KeyPressMsg{Code: tea.KeyEnter}}
	if m.mode == "speaker" {
		return []mouseControl{back}
	}
	switch m.mode {
	case "members":
		return []mouseControl{{i18n.MembersRefresh, mouseRuneKey('r')}, back}
	case "form":
		return []mouseControl{submit, back}
	case "chat-history-range":
		return []mouseControl{submit, back}
	case "selection":
		if m.selection != nil && m.selection.titleMode {
			return []mouseControl{submit, back}
		}
		return []mouseControl{back}
	case "pick", "help":
		return []mouseControl{back}
	case "cover", "cover-review":
		controls := []mouseControl{{i18n.LumenMousePreview, mouseRuneKey('p')}, {i18n.LumenMouseUpload, mouseRuneKey('u')}}
		if m.mode == "cover-review" && m.cover != nil {
			controls = append(controls, mouseControl{i18n.LumenMouseConfirm, tea.KeyPressMsg{Code: tea.KeyEnter}})
		}
		return append(controls, back)
	case "qr", "face":
		return []mouseControl{{i18n.LumenMouseOpenQR, mouseRuneKey('o')}, back}
	case "login-save":
		return []mouseControl{{i18n.LumenMouseRetry, tea.KeyPressMsg{Code: tea.KeyEnter}}, back}
	}
	return nil
}

// 同一组有序控件同时提供可见内容与点击区域。
func (m *Model) mouseControls() string {
	controls := m.mouseControlItems()
	if len(controls) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n")
	for i, control := range controls {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(m.theme.listRow(i18n.T(control.label), max(1, m.view.Width()), false))
	}
	return b.String()
}

// 与渲染器共用导航范围，只为可见标签生成点击区域。
func (m *Model) compactNavigationRange(width int) (start, end int) {
	names := pageNames()
	labelWidth := func(i int) int { return ansi.StringWidth(fmt.Sprintf(" %d %s ", (i+1)%10, names[i])) }
	start, end, used := m.page, m.page+1, labelWidth(m.page)
	if used >= width {
		return start, end
	}
	for start > 0 || end < len(names) {
		advanced := false
		if start > 0 && used+labelWidth(start-1) <= width {
			start--
			used += labelWidth(start)
			advanced = true
		}
		if end < len(names) && used+labelWidth(end) <= width {
			used += labelWidth(end)
			end++
			advanced = true
		}
		if !advanced {
			break
		}
	}
	return start, end
}

func (m *Model) rebuildMouseTargets(l workspaceLayout, content string) {
	m.mouseTargets = m.mouseTargets[:0]
	add := func(x, y, width int, kind string, index int, key tea.KeyPressMsg) {
		if y < 0 || y >= m.height || x >= m.width || width <= 0 {
			return
		}
		m.mouseTargets = append(m.mouseTargets, mouseTarget{x: x, y: y, width: min(width, m.width-x), kind: kind, index: index, key: key,
			page: m.page, mode: m.mode, editKind: m.editKind, screenWidth: m.width, screenHeight: m.height})
	}
	row := func(y int, kind string, index int, key tea.KeyPressMsg) {
		y -= m.view.YOffset()
		if y >= 0 && y < m.view.Height() {
			add(m.pickerLeft, m.pickerTop+y, m.view.Width(), kind, index, key)
		}
	}
	controls := m.mouseControlItems()
	controlStart := strings.Count(content, "\n") + 1 - len(controls)
	for i, control := range controls {
		row(controlStart+i, "key", 0, control.key)
	}
	if m.page == chatPage && m.mode == "" && m.chat != nil && l.chatTop > 0 {
		x := l.panelX + l.border + l.paddingX
		left := l.innerWidth
		for _, entry := range []struct {
			label string
			key   rune
		}{
			{i18n.T(i18n.MembersTitle) + " [u]", 'u'},
			{i18n.T(i18n.FleetTitle) + " [g]", 'g'},
			{i18n.T(i18n.ModerationAdmins) + " [m]", 'm'},
			{i18n.T(i18n.ModerationBlocks) + " [b]", 'b'},
			{i18n.T(i18n.DanmakuHistory) + " [h]", 'h'},
		} {
			width := ansi.StringWidth(entry.label)
			// 标题截断时省略号不属于入口，完整文字以外不响应点击。
			visible := ansi.StringWidth(ansi.Truncate(entry.label, max(0, left-1), ""))
			add(x, l.panelY+l.border+l.paddingY, visible, "chat", 0, mouseRuneKey(entry.key))
			x += width + 3
			left -= width + 3
		}
	}
	if m.mode == "chat-history" && l.chatTop > 0 {
		labelWidth := ansi.StringWidth("‹ " + strings.TrimSpace(i18n.T(i18n.LumenMouseBack)))
		add(l.panelX+l.border+l.paddingX, l.panelY+l.border+l.paddingY,
			min(l.innerWidth, labelWidth), "key", 0, tea.KeyPressMsg{Code: tea.KeyEsc})
	}
	if m.page == chatPage && (m.mode == "" || m.mode == "chat-history") && m.chat != nil {
		for row := 0; row < m.view.Height(); row++ {
			add(m.pickerLeft+m.view.Width(), m.pickerTop+row, 1, "chat-scroll", row, tea.KeyPressMsg{})
		}
	}
	if (m.busy && m.exitPrompt == nil) || m.previewing {
		return
	}
	if m.page == chatPage && m.mode == "" && m.chat != nil && l.chatComposer > 0 {
		for row := 0; row < l.chatComposer; row++ {
			add(m.pickerLeft, m.pickerTop+m.view.Height()+l.chatDivider+row,
				l.innerWidth-2*l.chatBorder, "chat-input", row, tea.KeyPressMsg{})
		}
	}
	if m.mode == "" {
		if l.rail > 0 {
			y := l.panelY + l.border + l.paddingY
			for i := range pageNames() {
				if i == 4 || i == 8 {
					y++
				}
				if y < l.panelY+l.panelHeight-l.border-l.paddingY {
					add(l.margin+l.border+l.paddingX, y, l.rail-2-2*l.border-2*l.paddingX, "page", i, tea.KeyPressMsg{})
				}
				y++
			}
		} else {
			start, end := m.compactNavigationRange(l.width)
			x := l.margin
			names := pageNames()
			for i := start; i < end; i++ {
				width := min(ansi.StringWidth(fmt.Sprintf(" %d %s ", (i+1)%10, names[i])), l.margin+l.width-x)
				add(x, lipgloss.Height(l.header)-1, width, "page", i, tea.KeyPressMsg{})
				x += width
			}
		}
		items := m.menu()
		if m.page != chatPage && len(items) > 0 {
			// pageContent 以操作行及一个换行符结束。
			start := strings.Count(strings.TrimSuffix(content, m.mouseControls()), "\n") - len(items)
			for i := range items {
				row(start+i, "action", i, tea.KeyPressMsg{})
			}
		}
		return
	}
	switch m.mode {
	case "speaker":
		start := lipgloss.Height(m.speakerIdentityView())
		for i := range 4 {
			row(start+i, "speaker", i, tea.KeyPressMsg{})
		}
	case "chat-history-range":
		row(3, "history-input", 0, tea.KeyPressMsg{})
		row(6, "history-input", 1, tea.KeyPressMsg{})
	case "form", "confirm":
		start := lipgloss.Height(m.confirmationPrompt())
		if m.mode == "form" {
			row(lipgloss.Height(m.formPrompt()), "input", 0, tea.KeyPressMsg{})
		} else {
			row(start, "confirm", 0, tea.KeyPressMsg{})
			row(start+1, "confirm", 1, tea.KeyPressMsg{})
		}
	case "pick":
		window := m.pickerWindow()
		start := max(0, m.selected-window+1)
		for i := start; i < min(len(m.choices), start+window); i++ {
			row(2+i-start, "pick", i, tea.KeyPressMsg{})
		}
	case "selection":
		if m.selection == nil {
			return
		}
		s := m.selection
		header, _, window := s.layout(m.view.Width(), m.view.Height())
		if s.titleMode && header > 0 {
			row(header-1, "title", 0, tea.KeyPressMsg{})
		}
		count := len(s.items)
		if s.titleMode {
			count = len(s.history)
		}
		start := max(0, s.selected-window+1)
		for i := start; i < min(count, start+window); i++ {
			row(header+i-start, "selection", i, tea.KeyPressMsg{})
		}
	}
}

func (m *Model) mouse(event tea.MouseMsg) tea.Cmd {
	position := event.Mouse()
	m.mouseX, m.mouseY, m.mouseKnown = position.X, position.Y, true
	if m.notificationMouse(event) {
		return nil
	}
	msg := event.Mouse()
	if _, wheel := event.(tea.MouseWheelMsg); wheel {
		if msg.Button != tea.MouseWheelUp && msg.Button != tea.MouseWheelDown {
			return nil
		}
		if m.previewing || msg.X < m.pickerLeft || msg.X >= m.pickerLeft+m.view.Width() || msg.Y < m.pickerTop || msg.Y >= m.pickerTop+m.view.Height() {
			return nil
		}
		delta := 1
		key := tea.KeyPressMsg{Code: tea.KeyDown}
		if msg.Button == tea.MouseWheelUp {
			delta = -1
			key.Code = tea.KeyUp
		}
		m.mouseTargets = nil
		if !m.busy {
			if m.mode == "pick" && len(m.choices) > 0 && m.selected+delta >= 0 && m.selected+delta < len(m.choices) {
				m.mouseScrolling = false
				return m.modalKey(key)
			}
			if m.mode == "selection" && m.selection != nil {
				s := m.selection
				first, count := 0, len(s.items)
				if s.titleMode {
					first, count = -1, len(s.history)
				}
				if s.selected+delta >= first && s.selected+delta < count {
					m.mouseScrolling = false
					return m.updateSelection(key)
				}
			}
		}
		m.mouseScrolling = true
		if m.page == chatPage && m.mode == "" {
			if handled, cmd := m.chatKey(key.String()); handled {
				return cmd
			}
		}
		m.view.SetYOffset(m.view.YOffset() + delta*3)
		return nil
	}
	if _, click := event.(tea.MouseClickMsg); !click || msg.Button != tea.MouseLeft {
		return nil
	}
	for _, target := range m.mouseTargets {
		if target.page != m.page || target.mode != m.mode || target.editKind != m.editKind || target.screenWidth != m.width || target.screenHeight != m.height || msg.Y != target.y || msg.X < target.x || msg.X >= target.x+target.width {
			continue
		}
		if m.previewing {
			return nil
		}
		if m.busy && m.exitPrompt == nil {
			if target.kind == "key" && target.key.Code == tea.KeyEsc {
				m.mouseTargets = nil
				_, cmd := m.Update(target.key)
				return cmd
			}
			return nil
		}
		m.mouseTargets = nil
		m.mouseScrolling = false
		switch target.kind {
		case "speaker":
			if m.speaker != nil {
				m.speaker.selected = target.index
				return m.chooseChatSpeaker()
			}
		case "page":
			m.chatInput.Blur()
			m.page = target.index
			m.view.GotoTop()
		case "action":
			m.cursors[m.page] = target.index
			return m.activate()
		case "pick":
			m.selected = target.index
			return m.choose()
		case "confirm":
			m.selected = target.index
			return m.modalKey(tea.KeyPressMsg{Code: tea.KeyEnter})
		case "input":
			return mouseFocusInput(&m.input, msg.X-target.x)
		case "chat-input":
			return m.focusChatInput(msg.X-target.x, target.index)
		case "history-input":
			b := m.chat.historyBrowser
			b.focus = target.index
			b.inputs[1-b.focus].Blur()
			return mouseFocusInput(&b.inputs[b.focus], msg.X-target.x)
		case "chat-scroll":
			m.view.SetYOffset(target.index * max(0, m.view.TotalLineCount()-m.view.Height()) / max(1, m.view.Height()-1))
			return nil
		case "title":
			if m.selection != nil {
				m.selection.selected = -1
				return mouseFocusInput(&m.selection.input, msg.X-target.x)
			}
		case "selection":
			if m.selection != nil {
				m.selection.selected = target.index
				return m.updateSelection(tea.KeyPressMsg{Code: tea.KeyEnter})
			}
		case "chat":
			m.chatInput.Blur()
			_, cmd := m.chatKey(target.key.String())
			return cmd
		case "key":
			if target.key.Code == tea.KeyEsc && m.obsBusy && m.mode == "" {
				_, cmd := m.Update(target.key)
				return cmd
			}
			if target.key.String() == "o" && (m.mode == "qr" || m.mode == "face") {
				return m.openQRImage()
			}
			return m.modalKey(target.key)
		}
		return nil
	}
	return nil
}

func mouseFocusInput(input *textinput.Model, column int) tea.Cmd {
	cmd := input.Focus()
	if input.EchoMode == textinput.EchoNone || input.Value() == "" {
		return cmd
	}
	visibleCursor, ok := textInputCursorColumn(*input)
	if !ok {
		return cmd
	}
	value := input.Value()
	position := input.Position()
	runeIndex, byteIndex := 0, len(value)
	for i := range value {
		if runeIndex == position {
			byteIndex = i
			break
		}
		runeIndex++
	}
	cursorCells := uniseg.StringWidth(value[:byteIndex])
	scale := 1
	if input.EchoMode == textinput.EchoPassword {
		// Bubbles 按显示单元格而非 rune 数量生成掩码。
		scale = uniseg.StringWidth(string(input.EchoCharacter))
		cursorCells *= scale
	}
	column = max(ansi.StringWidth(input.Styles().Focused.Prompt.Render(input.Prompt)), column)
	wanted := cursorCells + column - visibleCursor
	best, distance := 0, absMouseDistance(wanted)
	graphemes := uniseg.NewGraphemes(value)
	index, cells := 0, 0
	for graphemes.Next() {
		index += utf8.RuneCountInString(graphemes.Str())
		cells += graphemes.Width() * scale
		if d := absMouseDistance(cells - wanted); d < distance {
			best, distance = index, d
		}
	}
	input.SetCursor(best)
	return cmd
}

func absMouseDistance(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
