package tui

import (
	"fmt"
	"net/url"
	"strings"

	"arcana-world/internal/i18n"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) pageContent(width int) string {
	var b strings.Builder
	width = max(1, width)
	title := func(key i18n.Key) {
		b.WriteString(lipgloss.NewStyle().PaddingBottom(1).Render(m.theme.sectionTitle(i18n.T(key), width)) + "\n")
	}
	field := func(key i18n.Key, value string) {
		b.WriteString(m.theme.detailRow(i18n.T(key), value, width) + "\n")
	}
	note := func(text string) {
		b.WriteString(lipgloss.NewStyle().PaddingTop(1).Render(m.theme.hintText(strings.TrimSpace(text), width)) + "\n")
	}
	switch m.page {
	case livePage:
		title(i18n.TUILiveTitle)
		if m.room == nil {
			note(i18n.T(i18n.TUILiveRoomMissing))
		} else {
			field(i18n.LumenRoom, fmt.Sprint(m.room.ID))
			field(i18n.LumenTitle, clean(m.room.Title))
			field(i18n.LumenCategory, fmt.Sprintf("%s / %s (%d)", clean(m.room.ParentName), clean(m.room.AreaName), m.room.AreaID))
		}
		if m.stream != nil {
			field(i18n.LumenProtocol, m.stream.Protocol)
			if m.reveal {
				field(i18n.LumenStreamURL, clean(m.stream.Address))
				field(i18n.LumenStreamKey, clean(m.stream.Key))
			}
		}
		if !m.config.OBSAutoStream {
			note(i18n.T(i18n.TUILiveConfigureHint))
		}
	case accountsPage:
		title(i18n.TUIAccountsTitle)
	case roomPage:
		title(i18n.TUIRoomTitle)
		if m.room != nil {
			field(i18n.LumenTitle, clean(m.room.Title))
			field(i18n.LumenAnnouncement, clean(m.room.Announcement))
			field(i18n.LumenCover, clean(m.room.CoverURL))
			field(i18n.LumenReview, clean(m.room.CoverStatus))
		} else {
			note(i18n.T(i18n.TUILiveRoomMissing))
		}
	case obsPage:
		b.WriteString(lipgloss.NewStyle().PaddingBottom(1).Render(m.theme.sectionTitle("OBS Studio", width)) + "\n")
		field(i18n.LumenEndpoint, clean(m.config.OBSURL))
		state := m.theme.muted.Render(i18n.T(i18n.TUIOBSDisconnectedState))
		if m.obsState.Connecting || m.obsBusy {
			state = m.theme.accent.Render(i18n.T(i18n.TUIOBSWorkingState))
		} else if m.obsState.Connected {
			state = m.theme.positive.Render(i18n.T(i18n.TUIOBSConnectedState))
		}
		field(i18n.LumenConnection, state)
		if m.obsState.Connected {
			streaming := m.theme.muted.Render(i18n.T(i18n.TUIOBSStreamInactive))
			if m.obsState.Status.Active {
				streaming = m.theme.positive.Render(i18n.T(i18n.TUIOBSStreamActive))
			}
			if m.obsState.Status.Reconnecting {
				streaming = m.theme.warning.Render(i18n.T(i18n.TUIOBSStreamReconnecting))
			}
			field(i18n.LumenStream, streaming)
		}
	case overlayPage:
		title(i18n.OutputOverlayPage)
		b.WriteString(ansi.Wrap(strings.TrimSpace(m.overlayStateText()), width, "") + "\n")
		note(i18n.T(i18n.OutputOverlayDescription))
	case ttsPage:
		title(i18n.OutputTTSTitle)
		b.WriteString(ansi.Wrap(strings.TrimSpace(m.ttsStateText()), width, "") + "\n")
		field(i18n.LumenVoice, clean(m.ttsVoiceName()))
		field(i18n.LumenVolume, fmt.Sprintf("%d%%", m.config.TTS.Volume))
		note(i18n.T(i18n.LumenTTSHint))
	case settingsPage:
		title(i18n.TUISettingsTitle)
		proxy := m.config.Proxy
		if proxy == "" {
			proxy = i18n.T(i18n.TUISettingsSystemProxy)
		} else if u, e := url.Parse(proxy); e == nil && u.User != nil {
			u.User = url.User("***")
			proxy = u.String()
		}
		field(i18n.LumenProxy, clean(proxy))
		field(i18n.LumenProtocol, m.config.Protocol)
	case logsPage:
		title(i18n.TUILogsTitle)
		b.WriteString(m.theme.hintText(clean(m.journal.Path()), width) + "\n\n")
		if len(m.logs) == 0 {
			b.WriteString(m.theme.hintText(i18n.T(i18n.TUILogsEmpty), width))
		}
		for _, line := range m.logs {
			b.WriteString(ansi.Wrap(clean(line), width, "") + "\n")
		}
	case helpPage:
		title(i18n.TUIPageHelp)
		b.WriteString(m.theme.hintText(i18n.T(i18n.TUIHelpIntroduction), width) + "\n")
	case chatPage:
		return m.chatView(width)
	}
	items := m.menu()
	if len(items) > 0 {
		rows := make([]string, 0, len(items)+1)
		if m.page != helpPage {
			rows = append(rows, m.theme.sectionTitle(i18n.T(i18n.LumenActions), width))
		}
		cursor := min(m.cursors[m.page], len(items)-1)
		for i, item := range items {
			rows = append(rows, m.theme.listRow(clean(item.label), width, i == cursor))
		}
		b.WriteString(lipgloss.NewStyle().PaddingTop(1).Render(lipgloss.JoinVertical(lipgloss.Left, rows...)) + "\n")
	}
	return m.theme.paintSurface(m.theme.textStyle.Width(width).Render(b.String()), m.theme.surfaceColor)
}
