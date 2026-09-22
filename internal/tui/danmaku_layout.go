package tui

import (
	"fmt"
	"strings"

	"arcana-world/internal/i18n"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func renderedChatLines(content string) int { return strings.Count(content, "\n") + 1 }

// 滚动条按实际显示行数计算，消息换行和窗口缩放后仍保持准确。
func (m *Model) chatScrollView(records string) string {
	height, total := m.view.Height(), max(1, m.view.TotalLineCount())
	thumb := min(height, max(1, height*height/total))
	top := 0
	if total > height {
		top = m.view.YOffset() * (height - thumb) / (total - height)
	}
	bar := make([]string, height)
	for i := range bar {
		bar[i] = m.theme.muted.Render("│")
		if i >= top && i < top+thumb {
			bar[i] = m.theme.accent.Render("┃")
		}
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, records, strings.Join(bar, "\n"))
}
func (m *Model) chatPosition() string {
	above := m.view.YOffset()
	below := max(0, m.view.TotalLineCount()-above-m.view.Height())
	if above == 0 && below == 0 {
		return i18n.T(i18n.DanmakuAllVisible)
	}
	return fmt.Sprintf(i18n.T(i18n.DanmakuScrollPosition), above, below)
}
func (m *Model) chatWorkspace(l workspaceLayout, records string) string {
	body := m.chatScrollView(records)
	if l.chatPosition > 0 {
		body += "\n" + m.theme.muted.Render(ansi.Truncate(m.chatPosition(), l.innerWidth-2*l.chatBorder, "…"))
	}
	if l.chatComposer > 0 {
		if l.chatDivider > 0 {
			body += "\n" + m.theme.muted.Render(strings.Repeat("─", max(1, l.innerWidth-2*l.chatBorder)))
		}
		body += "\n" + m.chatComposerView(l)
	}
	if l.chatBorder > 0 {
		body = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(m.theme.muted.GetForeground()).Render(body)
	}
	if l.chatTop > 0 {
		header := m.chatHeader()
		if m.mode == "chat-history" {
			header = m.theme.accent.Render("‹ "+strings.TrimSpace(i18n.T(i18n.LumenMouseBack))) + " · " + m.chat.historyBrowser.rangeLabel()
		} else {
			header = m.theme.accent.Render(i18n.T(i18n.DanmakuHistory)+" [h]") + " · " + header
		}
		body = ansi.Truncate(header, l.innerWidth, "…") + "\n" + body
	}
	return body
}
