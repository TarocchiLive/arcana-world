package tui

import (
	"fmt"
	"strings"

	"arcana-world/internal/i18n"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/skip2/go-qrcode"
)

type workspaceLayout struct {
	width, margin, rail, panelWidth, panelHeight, paddingX, paddingY, border int
	panelX, panelY, innerWidth, innerHeight                                  int
	dialogX, dialogY, dialogWidth, dialogHeight                              int
	chatBorder, chatTop, chatComposer, chatDivider                           int
	header, footer                                                           string
}

func (m *Model) footerText() string {
	key := i18n.LumenFooterNavigate
	switch m.mode {
	case "chat-history":
		return i18n.T(i18n.DanmakuHistoryKeys)
	case "form":
		key = i18n.LumenFooterEdit
	case "pick", "selection":
		key = i18n.LumenFooterPick
	case "confirm":
		key = i18n.LumenFooterConfirm
	case "":
		if m.page == chatPage {
			key = i18n.LumenFooterChat
			if m.chatInputActive() {
				key = i18n.DanmakuInputKeys
			}
		}
	default:
		key = i18n.LumenFooterBack
	}
	if m.mode != "confirm" && (m.busy || (m.obsBusy && m.mode == "")) {
		key = i18n.LumenFooterBusy
	}
	return i18n.T(key)
}

func (m *Model) liveBadge() string {
	state, style := i18n.T(i18n.TUILiveUnknown), m.theme.muted
	if m.room != nil {
		state = i18n.T(i18n.TUILiveOffline)
		if m.room.Live {
			state, style = i18n.T(i18n.TUILiveOnline), m.theme.positive
		}
	}
	return style.Render("● " + state)
}

func (m *Model) obsBadge() string {
	state, style := i18n.T(i18n.TUIOBSDisconnectedState), m.theme.muted
	if m.obsState.Connecting || m.obsBusy {
		state, style = i18n.T(i18n.TUIOBSWorkingState), m.theme.warning
	} else if m.obsState.Connected {
		state, style = i18n.T(i18n.TUIOBSConnectedState), m.theme.positive
	}
	return style.Render("OBS " + state)
}

func (m *Model) workspaceHeader(width int) string {
	account := i18n.T(i18n.TUIAccountSignedOut)
	if m.account != nil {
		account = selectionText(m.account.Name) + " / " + m.account.UID
	}
	statusWidth := min(32, max(1, width/2))
	leftWidth := max(1, width-statusWidth-2)
	page := m.theme.selectedStyle.Padding(0, 1).Render(pageNames()[m.page])
	var status string
	if width < 48 {
		status = lipgloss.JoinVertical(lipgloss.Right,
			ansi.Truncate(page, statusWidth, "…"),
			ansi.Truncate(m.liveBadge(), statusWidth, "…"),
			ansi.Truncate(m.obsBadge(), statusWidth, "…"))
	} else {
		status = lipgloss.JoinVertical(lipgloss.Right,
			lipgloss.JoinHorizontal(lipgloss.Top,
				lipgloss.NewStyle().MarginRight(2).Render(page), m.liveBadge()),
			m.obsBadge())
	}
	status = lipgloss.NewStyle().Width(statusWidth).MaxWidth(statusWidth).
		Align(lipgloss.Right).Render(status)
	account = m.theme.muted.Render(ansi.Truncate(account, leftWidth, "…"))
	if m.showcaseVisible() {
		brand := lipgloss.JoinVertical(lipgloss.Left, m.wordmark(), account)
		return lipgloss.JoinVertical(lipgloss.Left,
			lipgloss.JoinHorizontal(lipgloss.Top,
				lipgloss.NewStyle().Width(width-statusWidth).Render(brand), status),
			m.showcaseSeparator(width))
	}
	brand := m.theme.accent.Render("Arcana") + m.theme.textStyle.Bold(true).Render(" WORLD")
	left := ansi.Truncate(brand, leftWidth, "…")
	if m.height >= 10 {
		left = lipgloss.JoinVertical(lipgloss.Left, left, account)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		lipgloss.NewStyle().Width(width-statusWidth).Render(left), status)
}

func (m *Model) statusBar(width int) string {
	inner := max(1, width-2)
	// 底栏只显示快捷键与操作进度，不随当前页面或连接状态改变高度。
	hints := m.theme.muted.Render(ansi.Truncate(m.footerText(), inner, "…"))
	body := hints
	if m.busy || m.obsBusy {
		progress := strings.Join(strings.Fields(clean(m.status)), " ")
		if progress == "" {
			progress = strings.TrimSpace(i18n.T(i18n.TUIStatusWorkingPrefix))
		}
		if remaining := inner - lipgloss.Width(hints) - 3; remaining > 0 {
			body = lipgloss.JoinHorizontal(lipgloss.Top, hints,
				m.theme.muted.Width(remaining+3).Align(lipgloss.Right).
					Render(ansi.Truncate(progress, remaining, "…")))
		}
	}
	style := lipgloss.NewStyle().Foreground(m.theme.textColor).Background(m.theme.elevatedColor).
		Width(width).Padding(0, min(1, max(0, width-1)))
	height := 1
	if m.height >= 12 {
		height = 2
		style = style.Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(m.theme.separatorColor).BorderBackground(m.theme.canvasColor)
	}
	return m.theme.paintSurface(style.Height(height).MaxHeight(height).MaxWidth(width).Render(body), m.theme.elevatedColor)
}

// 渲染、编辑器与鼠标命中区域共用同一套布局尺寸。
func (m *Model) workspace() workspaceLayout {
	if m.theme.id == "" || m.themeID != m.config.TUITheme {
		m.theme = themeFor(m.config.TUITheme, m.darkBackground)
		m.themeID = m.config.TUITheme
	}
	l := workspaceLayout{margin: 2, paddingX: 1}
	if m.width < 40 {
		l.margin = 1
	}
	if m.width < 8 {
		l.margin, l.paddingX = 0, 0
	}
	if m.width >= 24 && m.height >= 12 {
		l.border = 1
	}
	if m.height >= 20 {
		l.paddingY = 1
	}
	l.width = max(1, m.width-2*l.margin)
	if m.width >= 100 && m.height >= 24 {
		l.rail = 24
	}
	l.header = m.workspaceHeader(l.width)
	if l.rail == 0 {
		l.header = lipgloss.JoinVertical(lipgloss.Left, l.header, m.compactNavigation(l.width))
	}
	if m.height < 5 {
		l.header = m.compactNavigation(l.width)
	}
	l.footer = m.statusBar(l.width)
	l.panelX, l.panelY = l.margin+l.rail, lipgloss.Height(l.header)
	l.panelWidth = max(1, l.width-l.rail)
	l.panelHeight = max(1, m.height-l.panelY-lipgloss.Height(l.footer))
	l.innerWidth = max(1, l.panelWidth-2*l.border-2*l.paddingX)
	l.innerHeight = max(1, l.panelHeight-2*l.border-2*l.paddingY)
	m.view.SetWidth(l.innerWidth)
	m.view.SetHeight(l.innerHeight)
	m.pickerLeft = l.panelX + l.border + l.paddingX
	m.pickerTop = l.panelY + l.border + l.paddingY
	if m.page == chatPage && (m.mode == "" || m.mode == "chat-history") && m.chat != nil {
		if l.innerHeight >= 3 {
			l.chatTop = 1
		}
		if l.innerWidth >= 12 && l.innerHeight >= 8 {
			l.chatBorder = 1
		}
		if m.mode == "" && l.innerHeight >= 4 {
			m.styleChatInput()
			m.chatInput.SetWidth(max(1, l.innerWidth-2*l.chatBorder))
			rows := renderedChatLines(ansi.Wrap(m.chatInput.Value()+" ", max(1, m.chatInput.Width()-3), ""))
			l.chatComposer = min(max(1, rows), max(1, (l.innerHeight-2*l.chatBorder-l.chatTop)/3))
			m.chatInput.SetHeight(l.chatComposer)
			if l.innerHeight >= 8 {
				l.chatDivider = 1
			}
		}
		m.view.SetWidth(max(1, l.innerWidth-2*l.chatBorder-1))
		m.view.SetHeight(max(1, l.innerHeight-l.chatTop-l.chatComposer-l.chatDivider-2*l.chatBorder))
		m.pickerLeft += l.chatBorder
		m.pickerTop += l.chatTop + l.chatBorder
	}
	if m.mode == "chat-history-range" {
		m.resizeChatHistory()
	}
	if m.mode == "confirm" {
		l.dialogWidth = min(68, l.panelWidth)
		if l.panelWidth >= 36 {
			l.dialogWidth = min(l.dialogWidth, l.panelWidth-4)
		}
		m.view.SetWidth(max(1, l.dialogWidth-2*l.border-2*l.paddingX))
		desired := lipgloss.Height(m.modalContent()) + 2*l.border + 2*l.paddingY
		l.dialogHeight = min(l.panelHeight, desired)
		l.dialogX = l.panelX + (l.panelWidth-l.dialogWidth)/2
		l.dialogY = l.panelY + (l.panelHeight-l.dialogHeight)/2
		m.view.SetHeight(max(1, l.dialogHeight-2*l.border-2*l.paddingY))
		m.pickerLeft = l.dialogX + l.border + l.paddingX
		m.pickerTop = l.dialogY + l.border + l.paddingY
	}
	return l
}

func firstLines(value string, height int) string {
	lines := strings.Split(value, "\n")
	if len(lines) > max(1, height) {
		lines = lines[:max(1, height)]
	}
	return strings.Join(lines, "\n")
}

func (m *Model) syncWorkspace() {
	m.workspace()
	m.theme.styleInput(&m.input)
	resizeTextInput(&m.input, max(1, m.view.Width()-ansi.StringWidth(m.input.Prompt)-1))
	if m.selection != nil {
		m.selection.theme = m.theme
		m.theme.styleInput(&m.selection.input)
		m.selection.resize(m.view.Width(), m.view.Height())
	}
}

func (m *Model) compactNavigation(width int) string {
	names := pageNames()
	label := func(i int) string { return fmt.Sprintf("%d %s", (i+1)%10, names[i]) }
	active := label(m.page)
	if ansi.StringWidth(active)+2 >= width {
		return m.theme.selectedStyle.Padding(0, 1).MaxWidth(width).
			Render(ansi.Truncate(active, max(1, width-2), "…"))
	}
	start, end := m.compactNavigationRange(width)
	tabs := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		style := m.theme.muted
		if i == m.page {
			style = m.theme.selectedStyle
		}
		tabs = append(tabs, style.Padding(0, 1).Render(label(i)))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, tabs...)
}

func (m *Model) navigationRail(l workspaceLayout) string {
	width := max(1, l.rail-2-2*l.border-2*l.paddingX)
	height := max(1, l.panelHeight-2*l.border-2*l.paddingY)
	var lines []string
	for i, name := range pageNames() {
		if i == 4 || i == 8 {
			lines = append(lines, "")
		}
		style := m.theme.muted
		borderColor := m.theme.surfaceColor
		if i == m.page {
			style, borderColor = m.theme.selectedStyle, m.theme.accentColor
		}
		number := style.Width(3).Render(fmt.Sprint((i + 1) % 10))
		label := lipgloss.JoinHorizontal(lipgloss.Top, number,
			style.Render(ansi.Truncate(name, max(1, width-5), "…")))
		lines = append(lines, style.Width(width).PaddingLeft(1).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(borderColor).Render(label))
	}
	return m.theme.panel(firstLines(strings.Join(lines, "\n"), height), l.rail-2, l.panelHeight, l.paddingX, l.paddingY, l.border, false)
}

// 确认框使用独立视口；仅在没有可复用背景或终端尺寸变化时重绘底层页面。
func (m *Model) confirmationBackdrop(l workspaceLayout) string {
	view := viewport.New(viewport.WithWidth(l.innerWidth), viewport.WithHeight(l.innerHeight))
	view.SetContent(m.pageContent(l.innerWidth))
	return view.View()
}

func (m *Model) View() tea.View {
	if m.frame.reuse {
		m.frame.reuse = false
		m.refreshShowcaseFrame()
		return m.frameView()
	}
	l := m.workspace()
	if m.chat != nil {
		if m.page == chatPage && m.mode == "" {
			if !m.chat.shown && m.chat.historyBrowser == nil {
				m.chat.scrollToLatest = true
			}
			m.chat.shown = true
		} else {
			m.chat.shown = false
		}
	}
	poll := m.pollViewState()
	content := m.content()
	if m.mode == "chat-history" {
		b := m.chat.historyBrowser
		if b.viewportContent != content {
			m.view.SetContent(content)
			b.viewportContent = content
		}
	} else {
		m.view.SetContent(content)
	}
	cursor := m.contentCursor()
	if cursor != nil && !m.mouseScrolling {
		if cursor.Y < m.view.YOffset() {
			m.view.SetYOffset(cursor.Y)
		} else if cursor.Y >= m.view.YOffset()+m.view.Height() {
			m.view.SetYOffset(cursor.Y - m.view.Height() + 1)
		}
	}
	if !m.mouseScrolling && (m.mode == "pick" || m.mode == "confirm" || (m.mode == "" && len(m.menu()) > 0 && m.page != chatPage)) {
		for row, line := range strings.Split(content, "\n") {
			if !strings.HasPrefix(ansi.Strip(line), " › ") {
				continue
			}
			if row < m.view.YOffset() {
				m.view.SetYOffset(row)
			} else if row >= m.view.YOffset()+m.view.Height() {
				m.view.SetYOffset(row - m.view.Height() + 1)
			}
			break
		}
	}
	if m.page == chatPage && m.mode == "" && m.chat != nil && m.chat.scrollToLatest {
		m.view.GotoBottom()
		m.chat.scrollToLatest = false
	}
	m.rebuildMouseTargets(l, content)
	var screen string
	var textRegions [2]textRegion
	cachedBackdrop := m.mode == "confirm" && m.backdrop != "" && m.backdropWidth == m.width && m.backdropHeight == m.height
	if cachedBackdrop {
		screen = m.backdrop
	} else {
		body := m.view.View()
		textRegions[0] = m.pageTextRegion(body)
		if m.page == chatPage && (m.mode == "" || m.mode == "chat-history") && m.chat != nil {
			body = m.chatWorkspace(l, body)
		}
		if m.mode == "confirm" {
			body = m.confirmationBackdrop(l)
		}
		panel := m.theme.panel(body, l.panelWidth, l.panelHeight, l.paddingX, l.paddingY, l.border, m.mode != "" && m.mode != "confirm")
		if l.rail > 0 {
			rail := lipgloss.NewStyle().MarginRight(2).MarginBackground(m.theme.canvasColor).Render(m.navigationRail(l))
			panel = lipgloss.JoinHorizontal(lipgloss.Top, rail, panel)
		}
		screen = lipgloss.JoinVertical(lipgloss.Left, l.header, panel, l.footer)
		screen = m.theme.paintSurface(lipgloss.NewStyle().Foreground(m.theme.textColor).Background(m.theme.canvasColor).
			Width(max(1, m.width)).Height(max(1, m.height)).MaxWidth(max(1, m.width)).MaxHeight(max(1, m.height)).Padding(0, l.margin).Render(screen), m.theme.canvasColor)
		if m.mode != "confirm" {
			m.backdrop, m.backdropWidth, m.backdropHeight = screen, m.width, m.height
			m.backdropFooterHeight = lipgloss.Height(l.footer)
		}
	}
	var overlaySlots [4]floatingLayer
	layers := overlaySlots[:0]
	if m.mode == "confirm" {
		// 固定并淡化原页面，顶栏状态和确认操作的快捷键继续更新。
		height := lipgloss.Height(l.footer)
		if cachedBackdrop {
			height = max(height, m.backdropFooterHeight)
		}
		footer := m.theme.paintSurface(lipgloss.NewStyle().Width(m.width).Height(height).
			AlignVertical(lipgloss.Bottom).Background(m.theme.canvasColor).Padding(0, l.margin).Render(l.footer), m.theme.canvasColor)
		screen = m.theme.dim(screen)
		if cachedBackdrop {
			header := m.theme.paintSurface(lipgloss.NewStyle().Width(m.width).
				Background(m.theme.canvasColor).Padding(0, l.margin).Render(l.header), m.theme.canvasColor)
			layers = append(layers, floatingLayer{content: m.theme.dim(header), width: m.width, height: lipgloss.Height(l.header)})
		}
		layers = append(layers, floatingLayer{content: footer, y: m.height - height, width: m.width, height: height})
		dialog := m.theme.panel(m.view.View(), l.dialogWidth, l.dialogHeight, l.paddingX, l.paddingY, l.border, true)
		layers = append(layers, floatingLayer{content: dialog, x: l.dialogX, y: l.dialogY, width: l.dialogWidth, height: l.dialogHeight})
	}
	m.noticeLayer, textRegions[1] = m.notificationLayer(l.panelY, l.panelY+l.panelHeight)
	screen = composeLayer(screen, append(layers, m.noticeLayer)...)
	m.retainTextSelection(textRegions)
	m.frame = renderFrame{base: screen, poll: poll, cursor: m.screenCursor(l, cursor), textRegions: textRegions}
	if m.showcaseVisible() {
		m.frame.separator = floatingLayer{x: l.margin, y: lipgloss.Height(l.header) - 1, width: l.width, height: 1}
	}
	return m.frameView()
}

func (m *Model) pickerWindow() int { return max(1, m.view.Height()-5) }

func (m *Model) content() string {
	if m.mode != "" {
		return m.modalContent() + m.mouseControls()
	}
	return m.pageContent(m.view.Width()) + m.mouseControls()
}

func renderQR(link string) string {
	qr, err := qrcode.New(link, qrcode.Low)
	if err != nil {
		return i18n.T(i18n.TUIQRLinkTooLong)
	}
	bitmap := qr.Bitmap()
	rows := make([]string, 0, (len(bitmap)+1)/2)
	style := lipgloss.NewStyle().Foreground(lipgloss.Black).Background(lipgloss.White)
	// 明确指定黑白颜色，确保浅色和深色终端都有足够对比度。
	for y := 0; y < len(bitmap); y += 2 {
		var b strings.Builder
		for x, top := range bitmap[y] {
			bottom := false
			if y+1 < len(bitmap) {
				bottom = bitmap[y+1][x]
			}
			switch {
			case top && bottom:
				b.WriteRune('█')
			case top:
				b.WriteRune('▀')
			case bottom:
				b.WriteRune('▄')
			default:
				b.WriteByte(' ')
			}
		}
		rows = append(rows, style.Render(b.String()))
	}
	return lipgloss.JoinVertical(lipgloss.Left, rows...) + "\n"
}
