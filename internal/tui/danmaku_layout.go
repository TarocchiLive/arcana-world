package tui

import (
	"strings"

	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// 操作列表同时驱动侧栏和窄屏菜单，业务逻辑仍统一交给 chatKey。
func (m *Model) chatActions() []choice {
	follow := i18n.DanmakuPauseFollowing
	if !m.chat.follow {
		follow = i18n.DanmakuResumeFollowing
	}
	return []choice{
		{i18n.T(i18n.DanmakuOlderRecords), "["},
		{i18n.T(i18n.DanmakuNewerRecords), "]"},
		{i18n.T(i18n.DanmakuBackLatest), "end"},
		{i18n.T(follow), "space"},
		{toggleLabel(i18n.T(i18n.DanmakuToggle), !m.config.DanmakuDisabled), "s"},
		{toggleLabel(i18n.T(i18n.DanmakuOther), m.chat.showOther), "f"},
		{i18n.T(i18n.DanmakuRetryAction), "r"},
		{i18n.T(i18n.DanmakuSwitchRoom), "o"},
	}
}

func (m *Model) pickChatActions() tea.Cmd {
	m.chat.actionsOffset = m.view.YOffset()
	m.choices = m.chatActions()
	return m.pick("chat-actions", i18n.T(i18n.DanmakuActions))
}

func (m *Model) chooseChatAction(value string) tea.Cmd {
	m.restoreChatActions()
	_, cmd := m.chatKey(value)
	return cmd
}

// 菜单使用同一个视口，关闭时恢复消息的阅读位置。
func (m *Model) restoreChatActions() {
	if m.chat == nil {
		return
	}
	m.mode = ""
	m.workspace()
	m.view.SetContent(m.chatView(m.view.Width()))
	m.view.SetYOffset(m.chat.actionsOffset)
	m.chat.shown = true
}

// 坐标相对于面板内容区；仅为完整可见的操作行建立命中区域。
func (m *Model) chatActionLayout(l workspaceLayout) (string, []mouseTarget) {
	if m.chat == nil || m.previewing {
		return "", nil
	}
	actions := m.chatActions()
	width, start, height := l.chatActionsWidth-1, l.chatTop, l.innerHeight-l.chatTop
	if l.chatActionsWidth == 0 {
		actions = []choice{{i18n.T(i18n.DanmakuActions) + " [a]", "a"}}
		width, start, height = l.innerWidth, max(0, l.chatTop-1), min(1, l.chatTop)
	}
	kind := "chat"
	if m.busy || m.obsBusy {
		if m.cancel == nil && m.obsCancel == nil {
			return "", nil
		}
		actions = []choice{{i18n.T(i18n.LumenMouseBack), "esc"}}
		kind = "key"
	}
	var lines []string
	var targets []mouseTarget
	if l.chatActionsWidth > 0 {
		lines = append(lines, m.theme.sectionTitle(i18n.T(i18n.DanmakuActions)+" [a]", width))
		start++
		height--
	}
	for index, action := range actions {
		if index >= height {
			break
		}
		label := " " + strings.TrimSpace(action.label) + " "
		line := ansi.Truncate(label, max(1, width), "…")
		lines = append(lines, m.theme.accent.Render(line))
		key := tea.KeyPressMsg{}
		switch action.value {
		case "end":
			key.Code = tea.KeyEnd
		case "space":
			key.Code = tea.KeySpace
		case "esc":
			key.Code = tea.KeyEsc
		default:
			key = mouseRuneKey([]rune(action.value)[0])
		}
		targets = append(targets, mouseTarget{x: 0, y: start + index, width: min(width, ansi.StringWidth(label)), kind: kind, key: key})
	}
	return strings.Join(lines, "\n"), targets
}

func (m *Model) chatWorkspace(l workspaceLayout, records string) string {
	actions, _ := m.chatActionLayout(l)
	if l.chatBorder > 0 {
		records = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.muted.GetForeground()).
			Width(m.view.Width() + 2*l.chatBorder).Height(m.view.Height() + 2*l.chatBorder).Render(records)
	}
	var body string
	if l.chatActionsWidth > 0 {
		body = lipgloss.JoinHorizontal(lipgloss.Top, lipgloss.NewStyle().Width(l.chatActionsWidth).Render(actions), records)
	} else {
		if l.chatHeader == "" {
			status := ""
			if !m.view.AtBottom() || len(m.chat.newer) > 0 || m.chat.newMessages {
				status = i18n.T(i18n.DanmakuNewMessages)
			} else if m.chat.loading {
				status = i18n.T(i18n.DanmakuLoading)
			}
			if status != "" {
				actions = ansi.Truncate(actions+" · "+m.theme.muted.Render(status), l.innerWidth, "…")
			}
		}
		if l.chatTop > 0 {
			body = actions + "\n" + records
		} else {
			body = records
		}
	}
	if l.chatHeader != "" {
		more := !m.view.AtBottom() || len(m.chat.newer) > 0 || m.chat.newMessages
		header := ansi.Truncate(m.chatHeader(more), l.innerWidth, "…")
		if m.chat.loading {
			header = ansi.Truncate(m.theme.muted.Render(i18n.T(i18n.DanmakuLoading))+" · "+header, l.innerWidth, "…")
		}
		body = header + "\n" + body
	}
	return body
}
