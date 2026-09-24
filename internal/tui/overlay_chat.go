package tui

import (
	"slices"
	"strings"
	"unicode"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/presentation"
)

const (
	overlayChatLimit     = 6
	overlayChatTextLimit = 200
	overlayTitleLimit    = 200
)

// 浮层只投影主弹幕列表的快照，不单独读取或累积历史。
type overlayChatState struct {
	entries       []danmaku.Event
	disabled      []string
	text          string
	roles         string
	cached        bool
	cachedSummary overlaySummary
	cachedMode    string
	cachedChat    string
	cachedRoles   string
	content       string
	contentRoles  string
}

func (m *Model) syncOverlayChatSource() {
	c := &m.overlayChat
	var entries []danmaku.Event
	mode := m.config.Overlay.Content
	if m.overlayEnabled && !m.config.DanmakuDisabled && (mode == "danmaku" || mode == "combined") &&
		m.overlay != nil && (m.overlay.state == "starting" || m.overlay.state == "running") && m.chat != nil {
		entries = m.chat.entries
	}
	// 主列表每次接受查询结果都会替换快照；未变化时复用渲染文本。
	if len(c.entries) == 0 && len(entries) == 0 {
		return
	}
	if len(entries) > 0 && len(c.entries) == len(entries) && &c.entries[0] == &entries[0] && slices.Equal(c.disabled, m.config.OverlayDisabledEvents) {
		return
	}
	c.entries = entries
	c.disabled = slices.Clone(m.config.OverlayDisabledEvents)
	c.text, c.roles = "", ""
	if len(entries) == 0 {
		return
	}
	var lines [overlayChatLimit]string
	var roles [overlayChatLimit]byte
	count := 0
	for _, event := range entries {
		if event.Deleted || !presentation.Enabled(m.config.OverlayDisabledEvents, event) {
			continue
		}
		text := overlayChatPlain(presentation.RenderOverlay(event), overlayChatTextLimit)
		if text == "" {
			continue
		}
		lines[count] = text
		roles[count] = presentation.OverlayRole(event)
		count++
		if count == overlayChatLimit {
			break
		}
	}
	var b strings.Builder
	var orderedRoles [overlayChatLimit]byte
	for i := count - 1; i >= 0; i-- {
		if b.Len() != 0 {
			b.WriteByte('\n')
		}
		b.WriteString(lines[i])
		orderedRoles[count-1-i] = roles[i]
	}
	c.text, c.roles = b.String(), string(orderedRoles[:count])
}

func (m *Model) overlayContentText() string {
	m.syncOverlayChatSource()
	c := &m.overlayChat
	summary := m.overlaySummary()
	mode := m.config.Overlay.Content
	if c.cached && c.cachedSummary == summary && c.cachedMode == mode && c.cachedChat == c.text && c.cachedRoles == c.roles {
		return c.content
	}
	c.cached, c.cachedSummary, c.cachedMode, c.cachedChat = true, summary, mode, c.text
	c.cachedRoles = c.roles
	if mode == "danmaku" {
		c.content = c.text
		c.contentRoles = c.roles
		return c.content
	}
	summary.title = overlayChatPlain(summary.title, overlayTitleLimit)
	c.content = overlayText(summary)
	c.contentRoles = strings.Repeat("n", strings.Count(c.content, "\n")+1)
	if mode == "combined" && c.text != "" {
		c.content += "\n\n" + c.text
		c.contentRoles += "n" + c.roles
	}
	return c.content
}

// 移除控制及方向格式字符，将每个字段限制为有界的单行纯文本。
func overlayChatPlain(value string, limit int) string {
	var b strings.Builder
	count := 0
	for _, r := range value {
		if count == limit {
			b.WriteRune('…')
			break
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			if unicode.IsSpace(r) {
				r = ' '
			} else {
				continue
			}
		}
		if r == '\u2028' || r == '\u2029' {
			r = ' '
		}
		b.WriteRune(r)
		count++
	}
	return strings.TrimSpace(b.String())
}
