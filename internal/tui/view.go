package tui

import (
	"fmt"
	"net/url"
	"strings"

	"arcana-world/internal/i18n"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/skip2/go-qrcode"
)

var (
	accent        = lipgloss.NewStyle().Foreground(lipgloss.Color("81")).Bold(true)
	muted         = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	selectedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("232")).Background(lipgloss.Color("81")).Bold(true)
	warning       = lipgloss.NewStyle().Foreground(lipgloss.Color("215"))
	danger        = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true)
)

func (m *Model) View() string {
	width := max(12, m.width-4)
	account := i18n.T(i18n.TUIAccountSignedOut)
	if m.account != nil {
		account = clean(m.account.Name) + " / " + m.account.UID
	}
	liveState := i18n.T(i18n.TUILiveUnknown)
	liveStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("232")).Background(lipgloss.Color("220"))
	if m.room != nil {
		liveState = i18n.T(i18n.TUILiveOffline)
		liveStyle = liveStyle.Foreground(lipgloss.Color("255")).Background(lipgloss.Color("240"))
		if m.room.Live {
			liveState = i18n.T(i18n.TUILiveOnline)
			liveStyle = liveStyle.Background(lipgloss.Color("160"))
		}
	}
	header := liveStyle.Render("[ "+liveState+" ]") + "  " + accent.Render("ARCANA WORLD") + "  " + muted.Render("BILIBILI LIVE CONTROL")
	var tabs []string
	for i, p := range pageNames() {
		label := fmt.Sprintf(" %d %s ", (i+1)%10, p)
		if i == m.page {
			label = selectedStyle.Render(label)
		} else {
			label = muted.Render(label)
		}
		tabs = append(tabs, label)
	}
	tabBar := ansi.Wrap(strings.Join(tabs, ""), width, " ")
	chatHeader := ""
	if m.chat != nil {
		if m.page == chatPage && m.mode == "" {
			if !m.chat.shown && m.chat.follow {
				m.chat.scrollToLatest = true
			}
			m.chat.shown = true
			chatHeader = m.chatHeader(false)
		} else {
			m.chat.shown = false
		}
	}
	status := m.status
	if m.busy || m.obsBusy {
		status = i18n.T(i18n.TUIStatusWorkingPrefix) + status
	}
	footer := i18n.T(i18n.TUIFooterNavigation)
	if m.page == chatPage {
		footer = i18n.T(i18n.DanmakuControls)
	}
	if m.page == overlayPage || m.page == ttsPage {
		footer = i18n.T(i18n.OutputFooter)
	}
	if m.mode != "" {
		footer = i18n.T(i18n.TUIFooterModal)
	}
	if m.mode == "pick" && m.editKind == "output-events" {
		footer = i18n.T(i18n.OutputEventsFooter)
	}
	if m.busy {
		footer = i18n.T(i18n.TUIFooterBusy)
	}
	if m.obsBusy && !m.busy {
		footer = i18n.T(i18n.TUIFooterOBSBusy)
	}
	status = warning.Render(ansi.Wrap(clean(status), width, ""))
	m.view.Height = max(1, m.height-10-(lipgloss.Height(status)-1)-(lipgloss.Height(tabBar)-1))
	if chatHeader != "" {
		m.view.Height = max(1, m.view.Height-lipgloss.Height(chatHeader))
		chatHeader += "\n"
	}
	content := m.content()
	m.view.SetContent(content)
	if m.mode == "pick" || (m.mode == "" && len(m.menu()) > 0 && m.page != chatPage) {
		for row, line := range strings.Split(content, "\n") {
			if !strings.HasPrefix(ansi.Strip(line), " › ") {
				continue
			}
			if row < m.view.YOffset {
				m.view.SetYOffset(row)
			} else if row >= m.view.YOffset+m.view.Height {
				m.view.SetYOffset(row - m.view.Height + 1)
			}
			break
		}
	}
	if m.page == chatPage && m.mode == "" && m.chat != nil && m.chat.scrollToLatest {
		m.view.GotoBottom()
		m.chat.scrollToLatest = false
	}
	if chatHeader != "" {
		more := !m.view.AtBottom() || len(m.chat.newer) > 0 || m.chat.newMessages
		chatHeader = m.chatHeader(more) + "\n"
	}
	return lipgloss.NewStyle().Padding(1, 2).Render(
		ansi.Truncate(header, width, "") + "\n" +
			muted.Render(i18n.T(i18n.TUIViewAccountLabel)+account) + "\n" + tabBar + "\n\n" +
			chatHeader + m.view.View() + "\n" +
			status + "\n" +
			lipgloss.NewStyle().MaxWidth(width).Render(muted.Render(footer)))
}

// Keep page navigation aligned with the rows available below picker chrome.
func (m *Model) pickerWindow() int {
	return max(3, m.view.Height-5)
}

func (m *Model) content() string {
	switch m.mode {
	case "cover", "cover-review":
		return m.coverView()
	case "selection":
		return m.selection.View(m.view.Width, m.view.Height)
	case "form":
		if m.editKind == "cover-path" {
			return accent.Render(m.prompt) + "\n\n" + m.input.View() + i18n.T(i18n.TUIFormCoverControls)
		}
		promptStyle := accent
		if m.editKind == "clear-data" {
			promptStyle = danger
		}
		return promptStyle.Render(ansi.Wrap(clean(m.prompt), m.view.Width, "")) + "\n\n" + m.input.View() + "\n\n" + muted.Render(i18n.T(i18n.TUIFormControls))
	case "confirm":
		no, yes := i18n.T(i18n.TUIConfirmCancelLabel), i18n.T(i18n.TUIConfirmExecuteLabel)
		if m.selected == 0 {
			no = selectedStyle.Render(no)
		} else {
			yes = selectedStyle.Render(yes)
		}
		return warning.Render(ansi.Wrap(clean(m.prompt), m.view.Width, "")) + "\n\n" + no + "    " + yes + "\n\n" + muted.Render(i18n.T(i18n.TUIConfirmControls))
	case "pick":
		var b strings.Builder
		b.WriteString(accent.Render(m.prompt) + "\n\n")
		window := m.pickerWindow()
		start := max(0, m.selected-window+1)
		end := min(len(m.choices), start+window)
		for i := start; i < end; i++ {
			text := m.choices[i].label
			if m.editKind == "output-events" {
				text = m.outputEventLabel(m.choices[i])
			}
			label := ansi.Truncate(clean(text), max(1, m.view.Width-3), "…")
			if i == m.selected {
				b.WriteString(selectedStyle.Render(" › "+label) + "\n")
			} else {
				b.WriteString("   " + label + "\n")
			}
		}
		controls := i18n.TUISelectionPickerControls
		if m.editKind == "output-events" {
			controls = i18n.OutputEventsControls
		}
		fmt.Fprintf(&b, i18n.T(controls), m.selected+1, len(m.choices))
		return b.String()
	case "qr", "face":
		title := i18n.T(i18n.TUIQRSignInTitle)
		link := ""
		if m.qr != nil {
			link = m.qr.URL
		}
		if m.mode == "face" {
			title = i18n.T(i18n.TUIQRIdentityTitle)
			link = m.faceURL
		}
		return accent.Render(title) + "\n" + muted.Render(i18n.T(i18n.TUIQRVisibilityHint)) + "\n\n" + m.qrText + "\n" + clean(link) + "\n\n" + muted.Render(i18n.T(i18n.TUIQRTokenWarning))
	case "login-save":
		return warning.Render(i18n.T(i18n.TUILoginUnsavedTitle)) + i18n.T(i18n.TUILoginKeyringRetry)
	}
	var b strings.Builder
	switch m.page {
	case livePage:
		b.WriteString(accent.Render(i18n.T(i18n.TUILiveTitle)) + "\n\n")
		if m.room == nil {
			b.WriteString(i18n.T(i18n.TUILiveRoomMissing))
		} else {
			fmt.Fprintf(&b, i18n.T(i18n.TUILiveRoomDetails), m.room.ID, clean(m.room.Title), clean(m.room.ParentName), clean(m.room.AreaName), m.room.AreaID)
		}
		if m.stream != nil {
			fmt.Fprintf(&b, i18n.T(i18n.TUILiveProtocol), m.stream.Protocol)
			if m.reveal {
				fmt.Fprintf(&b, i18n.T(i18n.TUILiveStreamCredentials), clean(m.stream.Address), clean(m.stream.Key))
			}
		}
		if !m.config.OBSAutoStream {
			b.WriteString(i18n.T(i18n.TUILiveConfigureHint))
		}
	case accountsPage:
		b.WriteString(accent.Render(i18n.T(i18n.TUIAccountsTitle)) + i18n.T(i18n.TUIAccountsDescription))
	case roomPage:
		b.WriteString(accent.Render(i18n.T(i18n.TUIRoomTitle)) + "\n\n")
		if m.room != nil {
			fmt.Fprintf(&b, i18n.T(i18n.TUIRoomDetails), clean(m.room.Title), clean(m.room.Announcement), clean(m.room.CoverURL), clean(m.room.CoverStatus))
		}
		b.WriteString(i18n.T(i18n.TUIRoomCoverHint))
	case obsPage:
		b.WriteString(accent.Render("OBS STUDIO · WEBSOCKET V5") + "\n\n")
		fmt.Fprintf(&b, i18n.T(i18n.TUIOBSURLDetails), clean(m.config.OBSURL))
		state := i18n.T(i18n.TUIOBSDisconnectedState)
		if m.obsState.Connecting || m.obsBusy {
			state = i18n.T(i18n.TUIOBSWorkingState)
		} else if m.obsState.Connected {
			state = i18n.T(i18n.TUIOBSConnectedState)
		}
		fmt.Fprintf(&b, i18n.T(i18n.TUIOBSConnectionDetails), state)
		if m.obsState.Connected {
			streaming := i18n.T(i18n.TUIOBSStreamInactive)
			if m.obsState.Status.Active {
				streaming = i18n.T(i18n.TUIOBSStreamActive)
			}
			if m.obsState.Status.Reconnecting {
				streaming = i18n.T(i18n.TUIOBSStreamReconnecting)
			}
			fmt.Fprintf(&b, i18n.T(i18n.TUIOBSStreamDetails), streaming)
		}
		b.WriteString(i18n.T(i18n.TUIOBSDescription))
	case overlayPage:
		b.WriteString(accent.Render(i18n.T(i18n.OutputOverlayPage)) + "\n\n")
		b.WriteString(m.overlayStateText())
		b.WriteString(i18n.T(i18n.OutputOverlayDescription))
	case ttsPage:
		b.WriteString(accent.Render(i18n.T(i18n.OutputTTSTitle)) + "\n\n")
		b.WriteString(m.ttsStateText())
		fmt.Fprintf(&b, i18n.T(i18n.OutputTTSDescription), clean(m.ttsVoiceName()))
	case settingsPage:
		proxy := m.config.Proxy
		if proxy == "" {
			proxy = i18n.T(i18n.TUISettingsSystemProxy)
		} else if u, e := url.Parse(proxy); e == nil && u.User != nil {
			u.User = url.User("***")
			proxy = u.String()
		}
		fmt.Fprintf(&b, i18n.T(i18n.TUISettingsDetails), accent.Render(i18n.T(i18n.TUISettingsTitle)), clean(proxy), m.config.Protocol)
	case logsPage:
		b.WriteString(accent.Render(i18n.T(i18n.TUILogsTitle)) + "\n" + muted.Render(clean(m.journal.Path())) + "\n\n")
		if len(m.logs) == 0 {
			b.WriteString(i18n.T(i18n.TUILogsEmpty))
		}
		for _, line := range m.logs {
			b.WriteString(line + "\n")
		}
	case helpPage:
		b.WriteString(lipgloss.NewStyle().Width(m.view.Width).Render(helpText()))
	case chatPage:
		return m.chatView()
	}
	items := m.menu()
	if len(items) > 0 {
		b.WriteString("\n")
		cursor := min(m.cursors[m.page], len(items)-1)
		for i, item := range items {
			text := ansi.Truncate(clean(item.label), max(1, m.view.Width-3), "…")
			label := "   " + text
			if i == cursor {
				label = selectedStyle.Render(" › " + text)
			}
			b.WriteString(label + "\n")
		}
	}
	return b.String()
}
func renderQR(link string) string {
	qr, err := qrcode.New(link, qrcode.Low)
	if err != nil {
		return i18n.T(i18n.TUIQRLinkTooLong)
	}
	bitmap := qr.Bitmap()
	var b strings.Builder
	// 明确指定黑白颜色，确保浅色和深色终端都有足够对比度。
	for y := 0; y < len(bitmap); y += 2 {
		b.WriteString("\x1b[30;47m")
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
		b.WriteString("\x1b[0m\n")
	}
	return b.String()
}

func helpText() string {
	return i18n.T(i18n.TUIHelpGettingStarted)
}
