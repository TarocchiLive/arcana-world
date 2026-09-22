package tui

import (
	"fmt"
	"strings"
	"time"

	"arcana-world/internal/i18n"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const notificationDuration = 5 * time.Second

type notificationState struct {
	text       string
	warning    bool
	started    time.Time
	generation uint64
	pending    bool
	dismissed  bool
	scroll     int
}

type notificationExpired struct{ generation uint64 }

// 进度只更新底部状态，不覆盖尚未消失的操作结果。
func (m *Model) progressStatus(text string) {
	m.status = text
	m.statusWarning = false
}

func (m *Model) resetNotification() {
	if m.selectingNotice() {
		m.textSelection = nil
	}
	m.notification = notificationState{
		text:       m.status,
		warning:    m.statusWarning,
		started:    time.Now(),
		generation: m.notification.generation + 1,
		pending:    true,
	}
	m.noticeLayer = floatingLayer{}
}

// 每次新结果只安排一次计时，与背景动画和任务忙碌状态无关。
func (m *Model) notificationCommand() tea.Cmd {
	if !m.notification.pending || m.selectingNotice() {
		return nil
	}
	m.notification.pending = false
	if m.notification.dismissed || strings.TrimSpace(m.notification.text) == "" ||
		m.config.TUINotificationsWarningsOnly && !m.notification.warning {
		return nil
	}
	generation := m.notification.generation
	return tea.Tick(max(0, time.Until(m.notification.started.Add(notificationDuration))), func(time.Time) tea.Msg {
		return notificationExpired{generation: generation}
	})
}

func (m *Model) updateNotification(msg notificationExpired) tea.Cmd {
	if msg.generation == m.notification.generation && !m.selectingNotice() {
		m.notification.dismissed = true
		m.noticeLayer = floatingLayer{}
	}
	return nil
}

func (m *Model) notificationVisible() bool {
	return m.mode != "confirm" && strings.TrimSpace(m.notification.text) != "" && !m.notification.dismissed &&
		(!m.config.TUINotificationsWarningsOnly || m.notification.warning) &&
		(m.selectingNotice() || time.Since(m.notification.started) < notificationDuration)
}

// 渲染与鼠标滚动共用可见行数，并为滚动位置预留一行。
// 消息正文始终完整保留。
func (m *Model) notificationRows(width, height int) (lines []string, visible, offset int) {
	lines = strings.Split(ansi.Wrap(clean(m.notification.text), max(1, width-4), ""), "\n")
	visible = max(1, height-3)
	if len(lines) > visible {
		visible = max(1, visible-1)
	}
	offset = min(max(0, m.notification.scroll), max(0, len(lines)-visible))
	return
}

func (m *Model) notificationLayer(top, maxY int) (floatingLayer, textRegion) {
	if !m.notificationVisible() || m.width < 16 || maxY-top < 4 {
		return floatingLayer{}, textRegion{}
	}
	margin := 2
	if m.width < 40 {
		margin = 1
	}
	width := min(54, max(28, lipgloss.Width(clean(m.notification.text))+4), m.width-2*margin)
	maxHeight := min(12, maxY-top)
	lines, visible, offset := m.notificationRows(width, maxHeight)
	bodyColor, borderColor := m.theme.textColor, m.theme.separatorColor
	title := i18n.T(i18n.LumenNotificationInfo)
	if m.notification.warning {
		title = i18n.T(i18n.LumenNotificationWarning)
		bodyColor, borderColor = m.theme.warning.GetForeground(), m.theme.warning.GetForeground()
	}
	inner := width - 4
	style := lipgloss.NewStyle().Foreground(bodyColor).Background(m.theme.elevatedColor).Bold(m.notification.warning)
	heading := lipgloss.JoinHorizontal(lipgloss.Top,
		style.Bold(true).Width(inner-1).Render(ansi.Truncate(title, inner-2, "…")),
		style.Bold(true).Render("×"))
	rows := []string{heading}
	for _, line := range lines[offset:min(len(lines), offset+visible)] {
		rows = append(rows, style.Width(inner).Render(line))
	}
	body := strings.Join(rows[1:], "\n")
	bodyHeight := len(rows) - 1
	if len(lines) > visible {
		position := fmt.Sprintf("%d–%d/%d", offset+1, min(len(lines), offset+visible), len(lines))
		if ansi.StringWidth(position) > inner {
			position = fmt.Sprintf("%d/%d", offset+1, len(lines))
		}
		rows = append(rows, style.Foreground(m.theme.mutedColor).Width(inner).Align(lipgloss.Right).Render(ansi.Truncate(position, inner, "…")))
	}
	card := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor).
		BorderBackground(m.theme.elevatedColor).Background(m.theme.elevatedColor).
		Padding(0, 1).Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
	card = m.theme.paintSurface(card, m.theme.elevatedColor)
	layer := floatingLayer{content: card, x: m.width - margin - width, y: top, width: width, height: lipgloss.Height(card)}
	return layer, textRegion{kind: "notice", floatingLayer: floatingLayer{
		content: body, x: layer.x + 2, y: layer.y + 2, width: inner, height: bodyHeight,
	}}
}

func (m *Model) notificationMouse(msg tea.MouseMsg) bool {
	layer := m.noticeLayer
	mouse := msg.Mouse()
	if !m.notificationVisible() || layer.content == "" || mouse.X < layer.x || mouse.X >= layer.x+layer.width || mouse.Y < layer.y || mouse.Y >= layer.y+layer.height {
		return false
	}
	// 通知框内的所有鼠标事件都被拦截，防止穿透到下方的操作。
	switch msg.(type) {
	case tea.MouseClickMsg:
		if mouse.Button == tea.MouseLeft && mouse.Y == layer.y+1 && mouse.X == layer.x+layer.width-3 {
			m.dismissNotification()
		}
	case tea.MouseWheelMsg:
		if mouse.Button != tea.MouseWheelUp && mouse.Button != tea.MouseWheelDown {
			return true
		}
		lines, visible, offset := m.notificationRows(layer.width, layer.height)
		delta := 3
		if mouse.Button == tea.MouseWheelUp {
			delta = -delta
		}
		m.notification.scroll = min(max(0, offset+delta), max(0, len(lines)-visible))
	}
	return true
}

func (m *Model) dismissNotification() bool {
	if !m.notificationVisible() {
		return false
	}
	if m.selectingNotice() {
		m.clearTextSelection()
	}
	m.notification.dismissed = true
	m.noticeLayer = floatingLayer{}
	return true
}
