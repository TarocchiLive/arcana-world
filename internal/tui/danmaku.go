package tui

import (
	"errors"
	"time"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
)

const chatPageSize = 100

type danmakuUI struct {
	history        *danmaku.History
	listener       *danmaku.Listener
	err            error
	state          danmaku.Snapshot
	entries        []danmaku.Event
	rooms          []int64
	room           int64
	manualRoom     bool
	before         uint64
	newer          []uint64
	follow         bool
	showOther      bool
	loading        bool
	loaded         bool
	request        uint64
	revision       uint64
	seen, checked  uint64
	newMessages    bool
	scrollToLatest bool
	shown          bool
}
type chatTick struct{}
type chatPageMsg struct {
	request                uint64
	room                   int64
	before, revision       uint64
	entries                []danmaku.Event
	rooms                  []int64
	err                    error
	latest                 uint64
	newMessages, showOther bool
}

// Room changes discard read positions and unread state together. Keep any
// in-flight request: applyChatPage rejects its old room and starts a fresh read.
func (c *danmakuUI) resetRoom(room int64) {
	c.room = room
	c.before, c.newer, c.entries = 0, nil, nil
	c.follow, c.loaded = true, false
	c.seen, c.checked, c.newMessages = 0, 0, false
	c.scrollToLatest = false
}

func (m *Model) openChat() {
	if m.chat != nil && m.chat.history != nil {
		return
	}
	h, err := danmaku.Open(m.store.Dir())
	m.chat = &danmakuUI{history: h, err: err, follow: true}
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
	return errors.Join(m.chat.listener.Close(), m.chat.history.Close())
}
func chatTickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return chatTick{} })
}
func (m *Model) updateChat() tea.Cmd {
	if m.chat == nil {
		return chatTickCmd()
	}
	c := m.chat
	if c.listener != nil {
		c.state = c.listener.Snapshot()
		if !c.manualRoom && c.state.RoomID > 0 && c.room != c.state.RoomID {
			c.resetRoom(c.state.RoomID)
		}
	}
	var read tea.Cmd
	if m.page == chatPage && (!c.loaded || c.revision != c.state.Revision) {
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
	request, room, before, revision, h := c.request, c.room, c.before, c.state.Revision, c.history
	checked, newMessages, showOther := c.checked, c.newMessages, c.showOther
	return func() tea.Msg {
		rooms, err := h.Rooms()
		if err != nil {
			return chatPageMsg{request: request, room: room, before: before, showOther: showOther, err: err}
		}
		if room == 0 && len(rooms) > 0 {
			room = rooms[0]
		}
		var entries []danmaku.Event
		if room > 0 {
			entries, err = h.Page(room, before, chatPageSize)
		}
		var latest uint64
		if room > 0 && err == nil {
			if before == 0 {
				newMessages = false
				if len(entries) > 0 {
					latest = entries[0].Sequence
				}
			} else if !newMessages {
				latest, newMessages, err = newerChatEvents(h, room, checked, showOther)
			}
		}
		return chatPageMsg{request: request, room: room, before: before, revision: revision,
			entries: entries, rooms: rooms, err: err, latest: latest, newMessages: newMessages, showOther: showOther}
	}
}

// Check only arrivals not previously inspected. Hidden broadcasts cannot create
// the banner, and already-read newer pages are not mistaken for new arrivals.
func newerChatEvents(h *danmaku.History, room int64, after uint64, showOther bool) (uint64, bool, error) {
	var before, latest uint64
	limit := 1
	for {
		events, err := h.Page(room, before, limit)
		if err != nil {
			return 0, false, err
		}
		if len(events) == 0 {
			return latest, false, nil
		}
		if before == 0 {
			latest = events[0].Sequence
		}
		for _, event := range events {
			if event.Sequence <= after {
				return latest, false, nil
			}
			if chatEventVisible(event, showOther) {
				return latest, true, nil
			}
		}
		before = events[len(events)-1].Sequence
		limit = chatPageSize
	}
}

func (m *Model) applyChatPage(msg chatPageMsg) tea.Cmd {
	c := m.chat
	if c == nil || msg.request != c.request {
		return nil
	}
	c.loading = false
	if c.room != 0 && c.room != msg.room || c.before != msg.before || c.showOther != msg.showOther {
		return m.readChat()
	}
	c.err = msg.err
	if msg.err != nil {
		return nil
	}
	c.rooms, c.revision, c.loaded = msg.rooms, msg.revision, true
	if c.room == 0 {
		c.room = msg.room
	}
	if len(msg.entries) == 0 && msg.before != 0 && len(c.newer) > 0 {
		c.before = c.newer[len(c.newer)-1]
		c.newer = c.newer[:len(c.newer)-1]
		m.status = i18n.T(i18n.DanmakuNoOlder)
		return m.readChat()
	}
	c.entries = msg.entries
	if c.follow {
		c.seen, c.checked, c.newMessages = msg.latest, msg.latest, false
		c.scrollToLatest = true
	} else {
		c.checked = max(c.checked, msg.latest)
		c.newMessages = msg.newMessages
	}
	m.view.SetContent(m.content())
	return nil
}

func (m *Model) chatKey(key string) (bool, tea.Cmd) {
	c := m.chat
	if c == nil {
		return false, nil
	}
	switch key {
	case "f":
		c.showOther = !c.showOther
		c.checked, c.newMessages = c.seen, false
		return true, m.readChat()
	case "s":
		return true, m.perform("chat-toggle")
	case "r":
		if c.history == nil {
			m.openChat()
			c = m.chat
		}
		if c.listener != nil {
			phase := c.listener.Snapshot().Phase
			if phase == "error" || phase == "reconnecting" {
				c.listener.Retry()
			}
		}
		m.status = i18n.T(i18n.DanmakuRetry)
		return true, m.readChat()
	case " ", "space":
		c.follow = !c.follow
		c.scrollToLatest = false
		if c.follow {
			c.before, c.newer = 0, nil
			m.view.GotoTop()
		} else if c.before == 0 {
			// Freeze an exclusive high-water mark, including an empty history.
			c.before = 1
			if len(c.entries) > 0 {
				c.before = c.entries[0].Sequence + 1
			}
		}
		return true, m.readChat()
	case "[":
		if c.loading || len(c.entries) == 0 {
			return true, nil
		}
		if c.follow {
			c.before = c.entries[0].Sequence + 1
		}
		c.follow = false
		c.scrollToLatest = false
		c.newer = append(c.newer, c.before)
		c.before = c.entries[len(c.entries)-1].Sequence
		m.view.GotoTop()
		return true, m.readChat()
	case "]":
		if c.loading || len(c.newer) == 0 {
			return true, nil
		}
		c.before = c.newer[len(c.newer)-1]
		c.newer = c.newer[:len(c.newer)-1]
		m.view.GotoTop()
		return true, m.readChat()
	case "end":
		if c.loading {
			return true, nil
		}
		c.follow, c.manualRoom, c.before, c.newer = true, false, 0, nil
		if c.state.RoomID > 0 && c.room != c.state.RoomID {
			c.resetRoom(c.state.RoomID)
		}
		m.view.GotoTop()
		return true, m.readChat()
	case "o":
		if c.loading || len(c.rooms) == 0 {
			return true, nil
		}
		index := 0
		for i, room := range c.rooms {
			if room == c.room {
				index = (i + 1) % len(c.rooms)
				break
			}
		}
		c.resetRoom(c.rooms[index])
		c.manualRoom = true
		m.view.GotoTop()
		return true, m.readChat()
	}
	return false, nil
}
