package tui

import (
	"fmt"
	"strings"
	"time"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
)

const chatTimeFormat = "2006-01-02 15:04:05"

type chatHistoryBrowser struct {
	inputs                                 [2]textinput.Model
	focus                                  int
	start, end                             time.Time
	entries                                []danmaku.Event
	speakerSpans                           []chatSpeakerSpan
	returnView                             viewport.Model
	returnBottom                           bool
	loading                                bool
	request                                uint64
	err                                    error
	rendered, renderTheme, viewportContent string
	renderWidth                            int
}
type chatHistoryMsg struct {
	browser *chatHistoryBrowser
	request uint64
	entries []danmaku.Event
	err     error
}

func (b *chatHistoryBrowser) rangeLabel() string {
	return b.start.Format(chatTimeFormat) + " → " + b.end.Format(chatTimeFormat)
}
func (m *Model) openChatHistory() tea.Cmd {
	m.chatInput.Blur()
	b := &chatHistoryBrowser{returnView: m.view, returnBottom: m.view.AtBottom()}
	now := time.Now()
	for i := range b.inputs {
		b.inputs[i] = textinput.New()
		b.inputs[i].CharLimit = len(chatTimeFormat)
		b.inputs[i].SetValue(now.Add(time.Duration(i-1) * 24 * time.Hour).Format(chatTimeFormat))
		b.inputs[i].SetVirtualCursor(false)
	}
	m.chat.historyBrowser = b
	m.mode = "chat-history-range"
	m.view = viewport.New(viewport.WithWidth(m.view.Width()), viewport.WithHeight(m.view.Height()))
	return b.inputs[0].Focus()
}
func (m *Model) resizeChatHistory() {
	b := m.chat.historyBrowser
	if b == nil {
		return
	}
	for i := range b.inputs {
		m.theme.styleInput(&b.inputs[i])
		resizeTextInput(&b.inputs[i], max(1, m.view.Width()-3))
	}
}
func (m *Model) chatHistoryRangeView() string {
	b := m.chat.historyBrowser
	rows := []string{
		m.theme.sectionTitle(i18n.T(i18n.DanmakuHistory), m.view.Width()), "",
		m.theme.muted.Render(i18n.T(i18n.DanmakuHistoryStart)), b.inputs[0].View(), "",
		m.theme.muted.Render(i18n.T(i18n.DanmakuHistoryEnd)), b.inputs[1].View(), "",
		m.theme.hintText(i18n.T(i18n.DanmakuHistoryRangeKeys), m.view.Width()),
	}
	if b.err != nil {
		rows = append(rows, m.theme.warning.Render(m.safe(b.err.Error())))
	}
	return strings.Join(rows, "\n")
}
func (m *Model) chatHistoryView() string {
	b := m.chat.historyBrowser
	if b.loading {
		return m.theme.hintText(i18n.T(i18n.DanmakuLoading), m.view.Width())
	}
	if b.err != nil {
		return m.theme.warning.Render(m.safe(b.err.Error()))
	}
	if b.rendered == "" || b.renderWidth != m.view.Width() || b.renderTheme != m.theme.id {
		b.rendered = m.chatEventsView(b.entries, m.view.Width(), &b.speakerSpans)
		b.renderWidth, b.renderTheme = m.view.Width(), m.theme.id
	}
	return b.rendered
}
func (m *Model) closeChatHistory() {
	b := m.chat.historyBrowser
	m.mode = ""
	m.view = b.returnView
	m.chat.scrollToLatest = b.returnBottom
	m.chat.shown = true
	m.chat.historyBrowser = nil
}
func (m *Model) chatHistoryKey(msg tea.KeyPressMsg) tea.Cmd {
	b := m.chat.historyBrowser
	key := msg.String()
	if key == "esc" {
		if m.mode == "chat-history" {
			m.mode = "chat-history-range"
			m.view.GotoTop()
			return b.inputs[b.focus].Focus()
		}
		m.closeChatHistory()
		return nil
	}
	if m.mode == "chat-history" {
		m.view, _ = m.view.Update(msg)
		if key == "home" {
			m.view.GotoTop()
			return nil
		}
		if key == "end" {
			m.view.GotoBottom()
			return nil
		}
		return nil
	}
	switch key {
	case "up", "down", "tab", "shift+tab":
		next := 1 - b.focus
		if key == "up" {
			next = 0
		} else if key == "down" {
			next = 1
		}
		if next == b.focus {
			return nil
		}
		b.inputs[b.focus].Blur()
		b.focus = next
		return b.inputs[b.focus].Focus()
	case "enter":
		if b.focus == 0 {
			b.inputs[0].Blur()
			b.focus = 1
			return b.inputs[1].Focus()
		}
		return m.readChatHistory()
	}
	var cmd tea.Cmd
	b.inputs[b.focus], cmd = b.inputs[b.focus].Update(msg)
	return cmd
}
func (m *Model) readChatHistory() tea.Cmd {
	b := m.chat.historyBrowser
	start, err := time.ParseInLocation(chatTimeFormat, strings.TrimSpace(b.inputs[0].Value()), time.Local)
	end, endErr := time.ParseInLocation(chatTimeFormat, strings.TrimSpace(b.inputs[1].Value()), time.Local)
	if err != nil || endErr != nil || end.Before(start) {
		b.err = fmt.Errorf("%s", i18n.T(i18n.DanmakuHistoryRangeInvalid))
		return nil
	}
	// 时间输入精确到秒，查询需包含结束时间所在的完整一秒。
	b.start, b.end = start, end
	b.loading, b.err = true, nil
	b.viewportContent = ""
	b.request++
	request := b.request
	m.mode = "chat-history"
	b.inputs[b.focus].Blur()
	m.view.GotoTop()
	h, showOther := m.chat.history, m.config.DanmakuShowOther
	return func() tea.Msg {
		if h == nil {
			return chatHistoryMsg{browser: b, request: request, err: fmt.Errorf("%s", i18n.T(i18n.DanmakuError))}
		}
		entries, err := h.Range(start, end.Add(time.Second-time.Nanosecond), 0, func(e danmaku.Event) bool { return chatEventVisible(e, showOther) })
		return chatHistoryMsg{browser: b, request: request, entries: entries, err: err}
	}
}
func (m *Model) applyChatHistory(msg chatHistoryMsg) tea.Cmd {
	if m.chat == nil || m.chat.historyBrowser != msg.browser || msg.browser.request != msg.request {
		return nil
	}
	b := msg.browser
	b.entries, b.err, b.loading = msg.entries, msg.err, false
	b.rendered = ""
	return nil
}
