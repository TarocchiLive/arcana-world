package tui

import (
	"fmt"
	"slices"
	"strings"
	"unicode"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	"arcana-world/internal/presentation"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	overlayChatLimit        = 6
	overlayChatScanPages    = 4
	overlayChatScanPageSize = 64
	overlayChatTextLimit    = 200
	overlayTitleLimit       = 200
)

// Only bounded, already-sanitized lines survive a history read. Neither the
// viewport's room nor its pagination/read position participates in this state.
type overlayChatState struct {
	listener      *danmaku.Listener
	history       *danmaku.History
	account       string
	generation    uint64
	mode          string
	room          int64
	disabled      []string
	active        bool
	request       uint64
	loading       bool
	loaded        bool
	revision      uint64
	latest        uint64
	lines         [overlayChatLimit]string
	count         int
	readError     string
	text          string
	cached        bool
	cachedSummary overlaySummary
	cachedMode    string
	cachedChat    string
	content       string
}

type overlayChatMsg struct {
	request  uint64
	revision uint64
	latest   uint64
	lines    [overlayChatLimit]string
	count    int
	err      error
}

// syncOverlayChatSource is deliberately also called before rendering: an
// account, room or mode change clears published text before any async read.
func (m *Model) syncOverlayChatSource() {
	c := &m.overlayChat
	mode := m.config.Overlay.Content
	active := m.overlayEnabled && !m.config.DanmakuDisabled && (mode == "danmaku" || mode == "combined")
	active = active && m.overlay != nil && (m.overlay.state == "starting" || m.overlay.state == "running")
	var listener *danmaku.Listener
	var history *danmaku.History
	var account string
	var room int64
	var generation uint64
	if m.account != nil {
		account = m.account.UID
	}
	if active && account != "" && m.chat != nil {
		listener, history = m.chat.listener, m.chat.history
		if listener != nil && history != nil {
			state := listener.Snapshot()
			generation = state.Generation
			if state.AccountUID == account {
				room = state.RoomID
			}
		}
	}
	if c.listener == listener && c.history == history && c.account == account && c.generation == generation && c.mode == mode && c.room == room && c.active == active && slices.Equal(c.disabled, m.config.OverlayDisabledEvents) {
		return
	}
	request := c.request + 1
	*c = overlayChatState{listener: listener, history: history, account: account, generation: generation, mode: mode, disabled: slices.Clone(m.config.OverlayDisabledEvents), room: room, active: active, request: request}
}

func (m *Model) updateOverlayChat() tea.Cmd {
	m.syncOverlayChatSource()
	c := &m.overlayChat
	if !c.active || c.room <= 0 || c.listener == nil || c.history == nil || c.loading {
		return nil
	}
	revision := c.listener.Snapshot().Revision
	if c.loaded && c.revision == revision {
		return nil
	}
	c.loading = true
	c.request++
	request, room, after, history := c.request, c.room, c.latest, c.history
	disabled := c.disabled
	return func() tea.Msg {
		msg := overlayChatMsg{request: request, revision: revision, latest: after}
		var before uint64
		// A broadcast flood cannot turn a refresh into an unbounded
		// history scan. Previously collected chat lines remain available.
		for page := range overlayChatScanPages {
			events, err := history.Page(room, before, overlayChatScanPageSize)
			if err != nil {
				msg.err = err
				return msg
			}
			if len(events) == 0 {
				break
			}
			if page == 0 {
				msg.latest = max(after, events[0].Sequence)
			}
			for _, event := range events {
				if event.Sequence <= after {
					return msg
				}
				if event.Deleted || event.RoomID != room || !presentation.Enabled(disabled, event) {
					continue
				}
				text := overlayChatPlain(presentation.RenderOverlay(event), overlayChatTextLimit)
				if text == "" {
					continue
				}
				msg.lines[msg.count] = text
				msg.count++
				if msg.count == overlayChatLimit {
					return msg
				}
			}
			before = events[len(events)-1].Sequence
			if len(events) < overlayChatScanPageSize {
				break
			}
		}
		return msg
	}
}

func (m *Model) handleOverlayChat(msg overlayChatMsg) tea.Cmd {
	m.syncOverlayChatSource()
	c := &m.overlayChat
	if !c.loading || msg.request != c.request {
		return nil
	}
	c.loading = false
	if msg.err != nil {
		if detail := msg.err.Error(); detail != c.readError {
			c.readError = detail
			m.log(fmt.Sprintf(i18n.T(i18n.TUILogOverlayChatReadFailed), detail))
		}
		return nil
	}
	c.readError = ""
	c.loaded, c.revision, c.latest = true, msg.revision, msg.latest
	if msg.count == 0 {
		return nil
	}
	keep := min(c.count, overlayChatLimit-msg.count)
	copy(c.lines[msg.count:], c.lines[:keep])
	copy(c.lines[:msg.count], msg.lines[:msg.count])
	c.count = msg.count + keep
	var b strings.Builder
	for i := c.count - 1; i >= 0; i-- {
		if b.Len() != 0 {
			b.WriteByte('\n')
		}
		b.WriteString(c.lines[i])
	}
	c.text = b.String()
	return nil
}

func (m *Model) overlayContentText() string {
	m.syncOverlayChatSource()
	c := &m.overlayChat
	summary := m.overlaySummary()
	mode := m.config.Overlay.Content
	if c.cached && c.cachedSummary == summary && c.cachedMode == mode && c.cachedChat == c.text {
		return c.content
	}
	c.cached, c.cachedSummary, c.cachedMode, c.cachedChat = true, summary, mode, c.text
	if mode == "danmaku" {
		c.content = c.text
		return c.content
	}
	summary.title = overlayChatPlain(summary.title, overlayTitleLimit)
	c.content = overlayText(summary)
	if mode == "combined" && c.text != "" {
		c.content += "\n\n" + c.text
	}
	return c.content
}

// Strip terminal/control and directional format characters; each field is one
// bounded plain-text line, even if a wire payload contains embedded newlines.
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
