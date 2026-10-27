package tui

import (
	"net/url"
	"strconv"
	"strings"

	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// 设置行独立保存值与操作类型，不改变其他页面的菜单约定。
type settingsRow struct {
	label         i18n.Key
	value, action string
	group         i18n.Key
	hint          i18n.Key
	danger        bool
}

func settingSwitch(label i18n.Key, enabled bool, action string) settingsRow {
	return settingsRow{label: label, value: strings.TrimSpace(toggleLabel("", enabled)), action: action, hint: i18n.TUISettingsHintToggle}
}

func (m *Model) settingsRows(detail string) []settingsRow {
	if detail == "websocket" {
		return []settingsRow{
			settingSwitch(i18n.TUISettingsWebSocketEnabled, m.webSocketEnabled(), "websocket-toggle"),
			{label: i18n.TUISettingsWebSocketAddr, value: m.effectiveWebSocketAddr(), action: "websocket-addr", hint: i18n.TUISettingsHintEdit},
			{label: i18n.TUISettingsWebSocketToken, value: m.webSocketTokenStatus(), action: "websocket-token", hint: i18n.TUISettingsHintEdit},
		}
	}
	if detail == "appearance" {
		return []settingsRow{
			{label: i18n.LumenTheme, value: m.themeName(), action: "theme", hint: i18n.TUISettingsHintChoose},
			settingSwitch(i18n.LumenMotion, !m.config.TUIMotionDisabled, "motion"),
			settingSwitch(i18n.LumenCompactHeader, m.config.TUICompactHeader, "compact-header"),
			settingSwitch(i18n.LumenWarningsOnly, m.config.TUINotificationsWarningsOnly, "warnings-only"),
		}
	}
	proxy := m.config.Proxy
	if proxy == "" {
		proxy = i18n.T(i18n.TUISettingsSystemProxy)
	} else if u, err := url.Parse(proxy); err == nil && u.User != nil {
		u.User = url.User("***")
		proxy = u.String()
	}
	rows := []settingsRow{
		settingSwitch(i18n.DanmakuToggle, !m.config.DanmakuDisabled, "chat-toggle"),
		settingSwitch(i18n.DanmakuOther, m.config.DanmakuShowOther, "chat-other"),
		{label: i18n.DanmakuLimit, value: strconv.Itoa(m.config.DanmakuLimit), action: "chat-limit", hint: i18n.TUISettingsHintEdit},
		{label: i18n.LumenProtocol, value: m.config.Protocol, action: "protocol", hint: i18n.TUISettingsHintChoose},
		settingSwitch(i18n.TUISettingsExitOBSStop, !m.config.ExitOBSStopDisabled, "exit-obs-stop"),
		settingSwitch(i18n.TUISettingsExitLiveStop, !m.config.ExitLiveStopDisabled, "exit-live-stop"),
		{label: i18n.LumenAppearance, value: m.themeName() + " ›", action: "appearance", hint: i18n.TUISettingsHintOpen},
		{label: i18n.LumenProxy, value: proxy, action: "proxy", hint: i18n.TUISettingsHintEdit},
		{label: i18n.TUISettingsWebSocketTitle, value: m.settingsWebSocketState() + " ›", action: "websocket", hint: i18n.TUISettingsHintOpen},
		{label: i18n.TUISettingsReset, value: "…", action: "settings-reset-confirm", hint: i18n.TUISettingsHintConfirm},
		{label: i18n.TUISettingsClearData, value: "…", action: "clear-data", hint: i18n.TUISettingsHintConfirm, danger: true},
	}
	rows[0].group = i18n.TUISettingsGroupChat
	rows[3].group = i18n.TUISettingsGroupLive
	rows[6].group = i18n.TUISettingsGroupAppearance
	return rows
}

func (m *Model) settingsWebSocketState() string {
	key := i18n.TUISettingsWebSocketStopped
	if m.busy {
		key = i18n.TUIOBSWorkingState
	} else if m.websocket != nil {
		key = i18n.TUISettingsWebSocketRunning
	} else if m.webSocketEnabled() {
		key = i18n.TUISettingsWebSocketWaiting
	}
	return i18n.T(key)
}

func (m *Model) settingsMenu() []menuItem {
	rows := m.settingsRows("")
	items := make([]menuItem, len(rows))
	for i, row := range rows {
		items[i] = menuItem{i18n.T(row.label), row.action}
	}
	return items
}

func (m *Model) settingsRowView(row settingsRow, width int, selected bool) string {
	available := max(1, width-3)
	value := selectionText(row.value)
	arrow := row.hint == i18n.TUISettingsHintEdit || row.hint == i18n.TUISettingsHintChoose || row.hint == i18n.TUISettingsHintOpen
	if arrow {
		value = strings.TrimSuffix(value, " ›")
	}
	suffixWidth := 0
	if arrow {
		suffixWidth = 2
	}
	valueWidth := min(ansi.StringWidth(value)+suffixWidth, max(1, available/2))
	label := ansi.Truncate(i18n.T(row.label), max(1, available-valueWidth-1), "…")
	value = ansi.Truncate(value, max(0, valueWidth-suffixWidth), "…")
	if arrow {
		value += " ›"
	}
	text := label + strings.Repeat(" ", max(1, available-ansi.StringWidth(label)-ansi.StringWidth(value))) + value
	theme := m.theme
	if row.danger {
		theme.textStyle = theme.textStyle.Foreground(theme.danger.GetForeground())
		theme.selectedStyle = theme.selectedStyle.Foreground(theme.danger.GetForeground())
	}
	return theme.listRow(text, width, selected)
}

// 返回真实行号，渲染与鼠标共用分组间距，标题不会成为可点击项。
func (m *Model) settingsPageRows(width int) (string, []int) {
	lines := []string{m.theme.sectionTitle(i18n.T(i18n.TUISettingsTitle), width)}
	positions := make([]int, 0, 11)
	for i, row := range m.settingsRows("") {
		if row.action == "settings-reset-confirm" {
			lines = append(lines, "", m.theme.muted.Render(strings.Repeat("─", max(1, width))))
		}
		if row.group != "" {
			lines = append(lines, "", m.theme.sectionTitle(i18n.T(row.group), width))
		}
		positions = append(positions, len(lines))
		lines = append(lines, m.settingsRowView(row, width, i == m.cursors[settingsPage]))
	}
	return strings.Join(lines, "\n") + "\n", positions
}

func (m *Model) openSettingsDetail(detail string) tea.Cmd {
	m.settingsDetail = detail
	m.settingsDetailSelected = 0
	m.showSettingsDetail()
	if detail == "websocket" {
		return m.loadWebSocketTokenStatus()
	}
	return nil
}

func (m *Model) showSettingsDetail() tea.Cmd {
	title := i18n.LumenAppearance
	if m.settingsDetail == "websocket" {
		title = i18n.TUISettingsWebSocketTitle
	}
	m.choices = nil
	for _, row := range m.settingsRows(m.settingsDetail) {
		m.choices = append(m.choices, choice{i18n.T(row.label), row.action})
	}
	m.pick("settings-detail", i18n.T(title))
	m.selected = min(m.settingsDetailSelected, len(m.choices)-1)
	return nil
}

func (m *Model) chooseSettingsDetail() tea.Cmd {
	m.settingsDetailSelected = m.selected
	action := m.choices[m.selected].value
	if m.settingsDetail == "appearance" && action != "theme" {
		return m.toggleAppearance(action)
	}
	return m.perform(action)
}

func (m *Model) settingsHint() i18n.Key {
	rows, selected := m.settingsRows(""), m.cursors[settingsPage]
	if m.mode == "pick" && m.editKind == "settings-detail" {
		rows, selected = m.settingsRows(m.settingsDetail), m.selected
	}
	if selected >= 0 && selected < len(rows) {
		return rows[selected].hint
	}
	return i18n.LumenFooterNavigate
}
