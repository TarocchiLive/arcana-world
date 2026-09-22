package danmaku

import (
	"encoding/json"
	"strconv"
	"strings"
)

// 直播间事件仅映射收到的文本和明确指定的字段。
// 不为未记载的枚举值猜测含义。
func projectRoomEvent(p projection, cmd string, root map[string]any) projection {
	data := object(root["data"])
	e := p.event
	e.Text = ""
	switch cmd {
	case "NOTICE_MSG":
		text, ok := root["msg_common"].(string)
		if !ok {
			return p
		}
		if text == "" {
			text, ok = root["msg_self"].(string)
		}
		if !ok || text == "" {
			return p
		}
		e.Kind, e.Text = "notice", text
	case "LIKE_INFO_V3_NOTICE":
		segments, ok := data["content_segments"].([]any)
		if !ok || len(segments) == 0 {
			return p
		}
		var text strings.Builder
		for _, segment := range segments {
			s, ok := object(segment)["text"].(string)
			if !ok {
				return p
			}
			text.WriteString(s)
		}
		if text.Len() == 0 {
			return p
		}
		e.Kind, e.Text = "notice", text.String()
	case "FLOW_REWARD_CARD":
		name, nameOK := data["anchor_name"].(string)
		text, textOK := data["description"].(string)
		room, roomOK := roomEventInteger(data["room_id"], true)
		if !nameOK || !textOK || text == "" || !roomOK || room <= 0 {
			return p
		}
		e.Kind, e.User, e.Text, e.Count = "recommendation", name, text, room
	case "WATCHED_CHANGE", "LIKE_INFO_V3_UPDATE":
		field, kind := "num", "watched"
		if cmd == "LIKE_INFO_V3_UPDATE" {
			field, kind = "click_count", "likes"
		}
		count, ok := roomEventInteger(data[field], false)
		if !ok {
			return p
		}
		e.Kind, e.Count = kind, count
	case "ONLINE_RANK_COUNT":
		value, exists := data["online_count"]
		if !exists {
			value = data["count"]
		}
		count, ok := roomEventInteger(value, false)
		if !ok {
			return p
		}
		// online_count 是新版排行计数，并非观众人数。
		e.Kind, e.Count = "rank_count", count
	case "STOP_LIVE_ROOM_LIST":
		rooms, ok := data["room_id_list"].([]any)
		if !ok {
			return p
		}
		for _, value := range rooms {
			room, valid := roomEventInteger(value, true)
			if !valid || room <= 0 {
				return p
			}
			if room == e.RoomID {
				e.Amount = 1
			}
		}
		e.Kind, e.Count = "stop_rooms", int64(len(rooms))
	case "LIKE_INFO_V3_CLICK", "ROOM_BLOCK_MSG":
		name, ok := data["uname"].(string)
		uid, valid := roomEventInteger(data["uid"], true)
		if !ok || !valid || uid <= 0 {
			return p
		}
		e.Kind, e.User, e.UID = "like", name, stringValue(data["uid"])
		if cmd == "ROOM_BLOCK_MSG" {
			e.Kind = "room_block"
		}
	case "LIVE", "PREPARING":
		room, ok := roomEventInteger(root["roomid"], true)
		if !ok || room <= 0 {
			return p
		}
		e.Kind = "live"
		if cmd == "PREPARING" {
			e.Kind = "preparing"
		}
	case "ROOM_CHANGE":
		text, ok := data["title"].(string)
		if !ok {
			return p
		}
		e.Kind, e.Text = "room_change", text
	case "CUT_OFF":
		text, ok := root["msg"].(string)
		if !ok || text == "" {
			return p
		}
		e.Kind, e.Text = "cut_off", text
	default:
		return p
	}
	p.event = e
	return p
}

// 计数必须是 JSON 整数。线格式中的 ID 也可能采用十进制字符串。
// 与 number 不同，此处区分缺失或无效值与零值。
func roomEventInteger(value any, allowString bool) (int64, bool) {
	var text string
	switch value := value.(type) {
	case json.Number:
		text = string(value)
	case string:
		if !allowString {
			return 0, false
		}
		text = value
	default:
		return 0, false
	}
	if text == "" {
		return 0, false
	}
	for _, digit := range text {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	valueInt, err := strconv.ParseInt(text, 10, 64)
	return valueInt, err == nil
}
