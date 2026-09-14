package tui

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const chatPageSize = 100

type danmakuUI struct {
	history    *danmaku.History
	listener   *danmaku.Listener
	err        error
	state      danmaku.Snapshot
	entries    []danmaku.Event
	rooms      []int64
	room       int64
	manualRoom bool
	before     uint64
	newer      []uint64
	follow     bool
	showOther  bool
	loading    bool
	loaded     bool
	request    uint64
	revision   uint64
}
type chatTick struct{}
type chatPageMsg struct {
	request          uint64
	room             int64
	before, revision uint64
	entries          []danmaku.Event
	rooms            []int64
	err              error
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
			c.room = c.state.RoomID
			c.before, c.newer, c.entries = 0, nil, nil
			c.follow, c.loaded = true, false
			// An in-flight result is rejected by room; its completion schedules the new page.
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
	return func() tea.Msg {
		rooms, err := h.Rooms()
		if err != nil {
			return chatPageMsg{request: request, room: room, before: before, err: err}
		}
		if room == 0 && len(rooms) > 0 {
			room = rooms[0]
		}
		var entries []danmaku.Event
		if room > 0 {
			entries, err = h.Page(room, before, chatPageSize)
		}
		return chatPageMsg{request: request, room: room, before: before, revision: revision, entries: entries, rooms: rooms, err: err}
	}
}
func (m *Model) applyChatPage(msg chatPageMsg) tea.Cmd {
	c := m.chat
	if c == nil || msg.request != c.request {
		return nil
	}
	c.loading = false
	if c.room != 0 && c.room != msg.room || c.before != msg.before {
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
		return true, nil
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
		if c.state.RoomID > 0 {
			c.room = c.state.RoomID
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
		c.room, c.manualRoom, c.follow = c.rooms[index], true, true
		c.before, c.newer, c.entries = 0, nil, nil
		m.view.GotoTop()
		return true, m.readChat()
	}
	return false, nil
}

func (m *Model) chatStatus() string {
	if m.chat == nil {
		return i18n.T(i18n.DanmakuWaiting)
	}
	c := m.chat
	if c.history == nil {
		return i18n.T(i18n.DanmakuError)
	}
	key := i18n.DanmakuWaiting
	switch c.state.Phase {
	case "disabled":
		key = i18n.DanmakuDisabled
	case "connecting":
		key = i18n.DanmakuConnecting
	case "connected":
		key = i18n.DanmakuConnected
	case "reconnecting":
		key = i18n.DanmakuReconnecting
	case "storage_error":
		key = i18n.DanmakuStorageError
	case "error":
		key = i18n.DanmakuError
	}
	return i18n.T(key)
}
func (m *Model) chatView() string {
	if m.chat == nil {
		return i18n.T(i18n.DanmakuNoRoom)
	}
	c := m.chat
	var b strings.Builder
	b.WriteString(accent.Render(i18n.T(i18n.DanmakuPage)) + "\n")
	follow := i18n.T(i18n.DanmakuPaused)
	if c.follow {
		follow = i18n.T(i18n.DanmakuFollowing)
	}
	fmt.Fprintf(&b, i18n.T(i18n.DanmakuDetails), m.chatStatus(), c.room, follow, len(c.entries))
	if c.state.Err != nil {
		b.WriteString(warning.Render(m.safe(c.state.Err.Error())) + "\n")
	}
	b.WriteString(toggleLabel(i18n.T(i18n.DanmakuToggle), !m.config.DanmakuDisabled) + "\n")
	if c.err != nil {
		fmt.Fprintf(&b, i18n.T(i18n.DanmakuHistoryError), m.safe(c.err.Error()))
		b.WriteByte('\n')
	}
	if c.loading {
		b.WriteString(i18n.T(i18n.DanmakuLoading) + "\n")
	}
	if c.room == 0 {
		b.WriteString(i18n.T(i18n.DanmakuNoRoom) + "\n")
	}
	if len(c.entries) == 0 {
		b.WriteString(i18n.T(i18n.DanmakuEmpty) + "\n")
	}
	b.WriteByte('\n')
	// Newest first keeps the live tail visible at the top; [] pages are stable
	// sequence cursors, independent of incoming traffic.
	for _, e := range c.entries {
		switch e.Kind {
		case "unknown", "detail", "notice", "recommendation", "watched", "rank_count", "stop_rooms", "likes":
			if !c.showOther {
				continue
			}
		}
		line := chatEventText(e)
		fmt.Fprintf(&b, "%s  %s\n", e.Time.Local().Format("01-02 15:04:05"), line)
	}
	b.WriteString("\n" + toggleLabel(i18n.T(i18n.DanmakuOther), c.showOther))
	return lipgloss.NewStyle().Width(max(12, m.view.Width)).Render(b.String())
}
func chatEventText(e danmaku.Event) string {
	user, text := chatText(e.User, 128), chatText(e.Text, 2048)
	if e.Deleted {
		text = i18n.T(i18n.DanmakuDeleted)
	}
	switch e.Kind {
	case "chat":
		return user + "：" + text
	case "gift":
		return fmt.Sprintf(i18n.T(i18n.DanmakuGift), user, chatText(e.Gift, 128), e.Count, e.Amount, chatText(e.CoinType, 32))
	case "sc":
		return fmt.Sprintf(i18n.T(i18n.DanmakuSC), e.Amount, user, text)
	case "guard":
		return fmt.Sprintf(i18n.T(i18n.DanmakuGuard), user, chatText(e.Gift, 128), e.Count)
	case "enter":
		return fmt.Sprintf(i18n.T(i18n.DanmakuEnter), user)
	case "follow":
		return fmt.Sprintf(i18n.T(i18n.DanmakuFollow), user)
	case "share":
		return fmt.Sprintf(i18n.T(i18n.DanmakuShare), user)
	case "special_follow":
		return fmt.Sprintf(i18n.T(i18n.DanmakuSpecialFollow), user)
	case "mutual_follow":
		return fmt.Sprintf(i18n.T(i18n.DanmakuMutualFollow), user)
	case "like":
		return fmt.Sprintf(i18n.T(i18n.DanmakuLike), user)
	case "likes":
		return fmt.Sprintf(i18n.T(i18n.DanmakuLikes), e.Count)
	case "watched":
		return fmt.Sprintf(i18n.T(i18n.DanmakuWatched), e.Count)
	case "rank_count":
		return fmt.Sprintf(i18n.T(i18n.DanmakuRankCount), e.Count)
	case "notice":
		return fmt.Sprintf(i18n.T(i18n.DanmakuNotice), text)
	case "recommendation":
		return fmt.Sprintf(i18n.T(i18n.DanmakuRecommendation), user, e.Count, text)
	case "stop_rooms":
		if e.Amount == 1 {
			return i18n.T(i18n.DanmakuPreparing)
		}
		return fmt.Sprintf(i18n.T(i18n.DanmakuStopRooms), e.Count)
	case "live":
		return i18n.T(i18n.DanmakuLive)
	case "preparing":
		return i18n.T(i18n.DanmakuPreparing)
	case "room_change":
		return fmt.Sprintf(i18n.T(i18n.DanmakuRoomChange), text)
	case "room_block":
		return fmt.Sprintf(i18n.T(i18n.DanmakuRoomBlock), user)
	case "cut_off":
		return fmt.Sprintf(i18n.T(i18n.DanmakuCutOff), text)
	case "delete":
		return i18n.T(i18n.DanmakuDelete)
	case "detail":
		var b strings.Builder
		b.WriteByte('[')
		b.WriteString(chatText(i18n.T(i18n.Key(e.Title)), 128))
		b.WriteByte(']')
		for _, field := range e.Fields {
			if b.Len() >= 4096 {
				b.WriteString(i18n.T(i18n.DanmakuTruncated))
				break
			}
			b.WriteByte(' ')
			b.WriteString(chatText(field.Name, 64))
			b.WriteByte('=')
			b.WriteString(chatEnumText(e.Text, field))
		}
		return b.String()
	case "gap":
		switch e.Text {
		case "session_start":
			return i18n.T(i18n.DanmakuSessionStart)
		case "session_end":
			return i18n.T(i18n.DanmakuSessionEnd)
		case "connection_lost":
			return i18n.T(i18n.DanmakuConnectionLost)
		}
		return text
	default:
		return fmt.Sprintf(i18n.T(i18n.DanmakuUnknown), text)
	}
}

// Bound rendering work without changing the durable event. Control characters
// cannot inject terminal commands or forge a second timestamped record.
func chatText(value string, limit int) string {
	var b strings.Builder
	b.Grow(min(len(value), limit*3))
	count := 0
	for index, r := range value {
		if count == limit || index >= limit*4 {
			b.WriteString(i18n.T(i18n.DanmakuTruncated))
			break
		}
		if unicode.IsControl(r) {
			if r != '\n' && r != '\r' && r != '\t' {
				continue
			}
			r = ' '
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}
