package tui

import (
	"fmt"
	"strings"
	"unicode"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	"github.com/charmbracelet/lipgloss"
)

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
func (m *Model) chatHeader() string {
	if m.chat == nil {
		return i18n.T(i18n.DanmakuNoRoom)
	}
	c := m.chat
	var b strings.Builder
	b.WriteString(accent.Render(i18n.T(i18n.DanmakuPage)) + "\n")
	b.WriteString(toggleLabel(i18n.T(i18n.DanmakuOther), c.showOther) + "\n")
	if !c.follow && c.newMessages {
		b.WriteString(warning.Render(i18n.T(i18n.DanmakuNewMessages)) + "\n")
	}
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
	return lipgloss.NewStyle().Width(max(12, m.view.Width)).Render(strings.TrimSuffix(b.String(), "\n"))
}

func (m *Model) chatView() string {
	if m.chat == nil {
		return i18n.T(i18n.DanmakuNoRoom)
	}
	c := m.chat
	var b strings.Builder
	// Storage stays newest-first for stable exclusive cursors; render chronologically.
	for index := len(c.entries) - 1; index >= 0; index-- {
		e := c.entries[index]
		if !chatEventVisible(e, c.showOther) {
			continue
		}
		line := chatEventText(e)
		fmt.Fprintf(&b, "%s  %s\n", e.Time.Local().Format("01-02 15:04:05"), line)
	}
	if b.Len() == 0 {
		b.WriteString(i18n.T(i18n.DanmakuEmpty))
	}
	return lipgloss.NewStyle().Width(max(12, m.view.Width)).Render(b.String())
}

// This is a display allowlist, not an ingestion filter. Unknown/new event kinds
// and commands stay hidden even with Other enabled; durable history is unchanged.
func chatEventVisible(e danmaku.Event, showOther bool) bool {
	switch e.Kind {
	case "chat", "gift", "sc", "guard", "enter", "follow",
		"like", "live", "preparing",
		"room_change", "room_block", "cut_off", "delete", "gap":
		return true
	case "watched", "likes", "share", "special_follow":
		return showOther
	case "detail":
		switch e.Text {
		// Purchases, recalls and moderation in the listening room.
		// GUARD_MSG is a cross-room broadcast; GUIARD_MSG is a local notice.
		case "USER_TOAST_MSG", "GUIARD_MSG",
			"LIVE_OPEN_PLATFORM_SEND_GIFT", "LIVE_OPEN_PLATFORM_GUARD",
			"RECALL_DANMU_MSG", "WARNING", "ROOM_SILENT_ON", "ROOM_SILENT_OFF",
			"room_admin_entrance", "ROOM_ADMIN_REVOKE":
			return true
		// Local interaction effects and gift-combo summaries.
		case "WELCOME", "WELCOME_GUARD", "ENTRY_EFFECT", "USER_VIRTUAL_MVP",
			"EFFECT_DANMAKU_MSG", "DM_INTERACTION", "COMBO_SEND", "COMBO_END",
			// Room membership/statistics, excluding every ranking feed.
			"ROOM_ADMINS", "ROOM_REAL_TIME_MESSAGE_UPDATE", "ROOM_REAL_TIME_MESSAGE_UPDATE_V2",
			// Lotteries and red envelopes held in the listening room.
			"ANCHOR_LOT_CHECKSTATUS", "ANCHOR_LOT_START", "ANCHOR_LOT_END", "ANCHOR_LOT_AWARD",
			"POPULARITY_RED_POCKET_START", "POPULARITY_RED_POCKET_V2_START",
			"POPULARITY_RED_POCKET_NEW", "POPULARITY_RED_POCKET_WINNER_LIST",
			"POPULARITY_RED_POCKET_V2_WINNER_LIST",
			"DANMU_GIFT_LOTTERY_START", "DANMU_GIFT_LOTTERY_END", "DANMU_GIFT_LOTTERY_AWARD",
			// Current-room PK progress/results and voice/video connections.
			"PK_BATTLE_PRE", "PK_BATTLE_START", "PK_BATTLE_PROCESS", "PK_BATTLE_END",
			"PK_BATTLE_SETTLE", "PK_BATTLE_SETTLE_USER", "PK_BATTLE_GIFT",
			"PK_BATTLE_CRIT", "PK_BATTLE_SPECIAL_GIFT", "PK_BATTLE_MATCH_TIMEOUT",
			"PK_AGAIN", "PK_BATTLE_ENTRANCE", "PK_BATTLE_PRO_TYPE", "PK_BATTLE_RANK_CHANGE",
			"PK_BATTLE_VOTES_ADD", "PK_CLICK_AGAIN", "PK_END", "PK_INVITE_CANCEL",
			"PK_INVITE_FAIL", "PK_INVITE_INIT", "PK_INVITE_REFUSE",
			"PK_INVITE_SWITCH_CLOSE", "PK_INVITE_SWITCH_OPEN", "PK_LOTTERY_START",
			"PK_MATCH", "PK_MIC_END", "PK_PRE", "PK_PROCESS", "PK_SETTLE", "PK_START",
			"VOICE_JOIN_STATUS", "VOICE_JOIN_LIST", "VOICE_JOIN_ROOM_COUNT_INFO", "VOICE_JOIN_SWITCH",
			"VIDEO_CONNECTION_JOIN_START", "VIDEO_CONNECTION_MSG", "VIDEO_CONNECTION_JOIN_END":
			return showOther
		}
	}
	return false
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
