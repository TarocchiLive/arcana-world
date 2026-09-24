package tui

import (
	"errors"
	"time"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
)

const chatRefreshInterval = 500 * time.Millisecond

type danmakuUI struct {
	history               *danmaku.History
	listener              *danmaku.Listener
	err                   error
	state                 danmaku.Snapshot
	entries               []danmaku.Event
	speakerSpans          []chatSpeakerSpan
	loading, loaded       bool
	request, revision     uint64
	sinceRoom             int64
	sinceSequence         uint64
	since                 time.Time
	limit                 int
	showOther             bool
	scrollToLatest, shown bool
	historyBrowser        *chatHistoryBrowser
}
type chatTick struct{}
type chatPageMsg struct {
	request, revision uint64
	entries           []danmaku.Event
	err               error
	limit             int
	showOther         bool
	at                time.Time
}

func (m *Model) openChat() {
	if m.chat != nil && m.chat.history != nil {
		return
	}
	h, err := danmaku.Open(m.store.Dir())
	m.chat = &danmakuUI{history: h, err: err}
	if err == nil {
		m.chat.listener = danmaku.NewListener(m.ctx, h)
		m.syncChat()
	}
}
func (m *Model) syncChat() {
	if m.chat == nil || m.chat.listener == nil {
		return
	}
	m.chat.listener.Configure(m.account, m.config.Proxy, !m.config.DanmakuDisabled)
}
func (m *Model) closeChat() error {
	if m.chat == nil || m.chat.history == nil {
		return nil
	}
	var err error
	if m.chat.listener != nil {
		err = m.chat.listener.Close()
	}
	return errors.Join(err, m.chat.history.Close())
}
func chatTickCmd() tea.Cmd {
	return tea.Tick(chatRefreshInterval, func(time.Time) tea.Msg { return chatTick{} })
}
func (m *Model) updateChat() tea.Cmd {
	if m.chat == nil {
		return chatTickCmd()
	}
	c := m.chat
	if c.listener != nil {
		c.state = c.listener.Snapshot()
	}
	var read tea.Cmd
	expired := len(c.entries) > 0 && c.entries[len(c.entries)-1].Time.Before(time.Now().Add(-24*time.Hour))
	if !c.loaded || c.revision != c.state.Revision || c.limit != m.config.DanmakuLimit || c.showOther != m.config.DanmakuShowOther || expired {
		read = m.readChat()
	}
	return tea.Batch(chatTickCmd(), read)
}
func (m *Model) readChat() tea.Cmd {
	c := m.chat
	if c == nil || c.history == nil || c.loading {
		return nil
	}
	c.loading = true
	c.request++
	request, revision, h := c.request, c.state.Revision, c.history
	limit, showOther := m.config.DanmakuLimit, m.config.DanmakuShowOther
	if limit <= 0 {
		limit = 30
	}
	count := limit
	if !c.loaded {
		count = min(10, count)
	}
	at := time.Now()
	start := at.Add(-24 * time.Hour)
	if c.since.After(start) {
		start = c.since
	}
	since, sinceRoom, sinceSequence := c.since, c.sinceRoom, c.sinceSequence
	return func() tea.Msg {
		entries, err := h.Range(start, at, count, func(e danmaku.Event) bool {
			if e.Time.Equal(since) && (e.RoomID < sinceRoom || e.RoomID == sinceRoom && e.Sequence < sinceSequence) {
				return false
			}
			return chatEventVisible(e, showOther)
		})
		return chatPageMsg{request: request, revision: revision, entries: entries, err: err, limit: limit, showOther: showOther, at: at}
	}
}
func (m *Model) applyChatPage(msg chatPageMsg) tea.Cmd {
	c := m.chat
	if c == nil || msg.request != c.request {
		return nil
	}
	c.loading = false
	if msg.limit != m.config.DanmakuLimit || msg.showOther != m.config.DanmakuShowOther {
		return m.readChat()
	}
	c.err = msg.err
	if msg.err != nil {
		return nil
	}
	// 首次加载确定主框起点，后续刷新不补入更早的记录。
	if !c.loaded {
		c.since = msg.at
		if len(msg.entries) > 0 {
			oldest := msg.entries[len(msg.entries)-1]
			c.since, c.sinceRoom, c.sinceSequence = oldest.Time, oldest.RoomID, oldest.Sequence
		}
	}
	mainVisible := m.page == chatPage && m.mode == ""
	reader := &m.view
	follow := !c.loaded || (mainVisible && reader.AtBottom())
	if b := c.historyBrowser; b != nil {
		reader, follow = &b.returnView, b.returnBottom
	} else if s := m.members; s != nil {
		reader, follow = &s.returnView, s.returnBottom
	} else if s := m.speaker; s != nil && s.parentMode == "" {
		reader, follow = &s.parentView, s.parentView.AtBottom()
	}
	mainReader := mainVisible || reader != &m.view
	offset := reader.YOffset()
	// 主框移除最早记录时，保持当前阅读位置。
	if mainReader && !follow && len(msg.entries) > 0 {
		oldest := msg.entries[len(msg.entries)-1]
		for i, e := range c.entries {
			if e.RoomID == oldest.RoomID && e.Sequence == oldest.Sequence {
				if i+1 < len(c.entries) {
					offset -= renderedChatLines(m.chatEventsView(c.entries[i+1:], reader.Width(), nil))
				}
				break
			}
		}
	}
	c.entries, c.revision, c.loaded = msg.entries, msg.revision, true
	c.limit, c.showOther = msg.limit, msg.showOther
	if mainReader {
		reader.SetContent(m.chatEventsView(c.entries, reader.Width(), &c.speakerSpans))
		reader.SetYOffset(max(0, offset))
		if follow {
			reader.GotoBottom()
		}
		c.scrollToLatest = follow
	}
	return nil
}
func (m *Model) chatKey(key string) (bool, tea.Cmd) {
	if key == "u" {
		return true, m.openRoomMembers(false)
	}
	if key == "g" {
		return true, m.openRoomMembers(true)
	}
	if key == "m" {
		return true, m.openModeration(moderationAdmins)
	}
	if key == "b" {
		return true, m.openModeration(moderationBlocks)
	}
	if m.chat == nil {
		return false, nil
	}
	switch key {
	case "/":
		return true, m.chatInput.Focus()
	case "h":
		return true, m.openChatHistory()
	case "end":
		m.view.GotoBottom()
		return true, nil
	case "home":
		m.view.GotoTop()
		return true, nil
	case "r":
		if m.chat.history == nil {
			m.openChat()
		}
		if m.chat.listener != nil {
			m.chat.listener.Retry()
		}
		m.progressStatus(i18n.T(i18n.DanmakuRetry))
		return true, m.readChat()
	}
	return false, nil
}
