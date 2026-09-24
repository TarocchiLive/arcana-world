package tui

import (
	"fmt"
	"strings"
	"unicode"

	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	"arcana-world/internal/presentation"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) chatStatus() string {
	if m.chat == nil {
		return i18n.T(i18n.DanmakuWaiting)
	}
	c := m.chat
	if c.err != nil {
		return fmt.Sprintf(i18n.T(i18n.DanmakuHistoryError), m.safe(c.err.Error()))
	}
	if c.history == nil {
		return i18n.T(i18n.DanmakuError)
	}
	key := i18n.DanmakuWaiting
	switch c.state.Phase {
	case danmaku.PhaseDisabled:
		key = i18n.DanmakuDisabled
	case danmaku.PhaseConnecting:
		key = i18n.DanmakuConnecting
	case danmaku.PhaseConnected:
		key = i18n.DanmakuConnected
	case danmaku.PhaseReconnecting:
		key = i18n.DanmakuReconnecting
	case danmaku.PhaseStorageError:
		key = i18n.DanmakuStorageError
	case danmaku.PhaseError:
		key = i18n.DanmakuError
	}
	return i18n.T(key)
}
func (m *Model) chatHeader() string {
	return m.theme.muted.Render(m.chatStatus())
}

func (m *Model) chatView(width int) string {
	if m.chat == nil {
		return m.theme.hintText(i18n.T(i18n.DanmakuEmpty), max(1, width))
	}
	return m.chatEventsView(m.chat.entries, width, &m.chat.speakerSpans)
}

func (m *Model) chatEventsView(entries []danmaku.Event, width int, target *[]chatSpeakerSpan) string {
	width = max(1, width)
	surface := lipgloss.NewStyle().Background(m.theme.surfaceColor).Width(width)
	var b strings.Builder
	var spans []chatSpeakerSpan
	// 点击区域归属于实际渲染的数据，测量行高时不写入任何视图。
	if target != nil {
		spans = (*target)[:0]
	}
	row := 0
	for index := len(entries) - 1; index >= 0; index-- {
		e := entries[index]
		line := chatEventText(e)
		style := m.theme.textStyle
		switch e.Kind {
		case "sc", "guard":
			style = m.theme.accent
		case "gift", "follow", "mutual_follow", "special_follow", "live":
			style = m.theme.positive
		case "cut_off", "room_block":
			style = m.theme.warning
		case "gap":
			style = m.theme.muted
			if e.Text == "connection_lost" {
				style = m.theme.warning
			}
		case "enter", "like", "likes", "watched", "detail", "preparing":
			style = m.theme.muted
		}
		if e.Kind == "chat" {
			user := chatText(e.User, 128)
			boundary := ""
			if e.UID != "" && e.UID != "0" && !e.Mystery && user != "" {
				boundary = chatSpeakerBoundary
			}
			line = m.theme.textStyle.Bold(true).Render(user) + boundary + m.theme.textStyle.Render(line[len(user):])
		} else {
			line = style.Render(line)
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		stamp := m.theme.muted.Render(e.Time.Local().Format("01-02 15:04:05"))
		textWidth := width
		if width >= 32 {
			textWidth -= 16
		}
		line = ansi.Wrap(line, textWidth, "")
		if end := strings.Index(line, chatSpeakerBoundary); end >= 0 {
			nameRow, column := row, 16
			if width < 32 {
				nameRow += strings.Count(ansi.Wrap(stamp, width, ""), "\n") + 1
				column = 0
			}
			for offset, part := range strings.Split(line[:end], "\n") {
				if cells := ansi.StringWidth(part); target != nil && cells > 0 {
					spans = append(spans, chatSpeakerSpan{event: e, row: nameRow + offset, column: column, width: cells})
				}
			}
			line = strings.ReplaceAll(line, chatSpeakerBoundary, "")
		}
		row += strings.Count(line, "\n") + 1
		if width < 32 {
			row += strings.Count(ansi.Wrap(stamp, width, ""), "\n") + 1
		}
		if width >= 32 {
			b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top,
				m.theme.muted.Width(16).Render(stamp),
				lipgloss.NewStyle().Width(width-16).Render(line)))
		} else {
			b.WriteString(lipgloss.JoinVertical(lipgloss.Left,
				ansi.Wrap(stamp, width, ""), lipgloss.NewStyle().Width(width).Render(line)))
		}
	}
	if target != nil {
		*target = spans
	}
	if b.Len() == 0 {
		b.WriteString(m.theme.hintText(i18n.T(i18n.DanmakuEmpty), width))
	}
	return surface.Render(b.String())
}

// 这是显示白名单而非入库过滤。即使开启其他消息，未知或新增事件
// 及命令仍保持隐藏，持久化历史不受影响。
func chatEventVisible(e danmaku.Event, showOther bool) bool {
	if presentation.Enabled(nil, e) {
		return true
	}
	switch e.Kind {
	case "gap", "watched", "likes", "share", "special_follow":
		return showOther
	case "detail":
		switch e.Text {
		// 本地互动特效与礼物连击汇总。
		case "WELCOME", "WELCOME_GUARD", "ENTRY_EFFECT", "USER_VIRTUAL_MVP",
			"EFFECT_DANMAKU_MSG", "DM_INTERACTION", "COMBO_SEND", "COMBO_END",
			// 房间成员与统计数据，不包含任何榜单推送。
			"ROOM_ADMINS", "ROOM_REAL_TIME_MESSAGE_UPDATE", "ROOM_REAL_TIME_MESSAGE_UPDATE_V2",
			// 当前监听房间的抽奖与红包。
			"ANCHOR_LOT_CHECKSTATUS", "ANCHOR_LOT_START", "ANCHOR_LOT_END", "ANCHOR_LOT_AWARD",
			"POPULARITY_RED_POCKET_START", "POPULARITY_RED_POCKET_V2_START",
			"POPULARITY_RED_POCKET_NEW", "POPULARITY_RED_POCKET_WINNER_LIST",
			"POPULARITY_RED_POCKET_V2_WINNER_LIST",
			"DANMU_GIFT_LOTTERY_START", "DANMU_GIFT_LOTTERY_END", "DANMU_GIFT_LOTTERY_AWARD",
			// 当前房间的 PK 进度、结果与音视频连线。
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
		count, unit := e.GuardPeriod()
		duration := chatText(unit, 32)
		if count > 0 {
			key := i18n.DanmakuGuardMonths
			switch unit {
			case "年":
				key = i18n.DanmakuGuardYears
			case "天":
				key = i18n.DanmakuGuardDays
			}
			duration = fmt.Sprintf(i18n.T(key), count)
		}
		if duration != "" {
			duration = fmt.Sprintf(i18n.T(i18n.DanmakuGuardDuration), duration)
		}
		return fmt.Sprintf(i18n.T(i18n.DanmakuGuard), user, duration, chatText(e.Gift, 128))
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

// 限制渲染开销，不改动持久化事件。控制字符不能注入终端命令，
// 也不能伪造带独立时间戳的第二条记录。
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
