// Package danmaku 保留收到的业务事件，并仅映射含义明确的字段。
package danmaku

import (
	"bytes"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"time"
)

// Event 是用于显示的映射结果；原始线格式数据单独存储。
// Time 为本地接收时间。礼物 Amount 使用原始 CoinType 单位；SC 的单位为元。
type Event struct {
	Sequence                              uint64
	RoomID                                int64
	Kind, User, UID, Text, Gift, CoinType string
	Count                                 int64
	Amount                                int64
	Face                                  string `json:",omitempty"`
	MedalName                             string `json:",omitempty"`
	MedalLevel                            int64  `json:",omitempty"`
	Mystery                               bool   `json:",omitempty"`
	GuardUnit                             string `json:",omitempty"`
	Time                                  time.Time
	Deleted                               bool
	Title                                 string       `json:",omitempty"`
	Fields                                []EventField `json:",omitempty"`
}

// GuardPeriod 返回时长，而非礼物数量。自带数量的单位
// （如 "*3天"）优先于协议中的数量。
func (e Event) GuardPeriod() (int64, string) {
	count, unit := e.Count, e.GuardUnit
	if e.Kind == "detail" {
		countKey, unitKey := "num", "unit"
		if e.Text == "LIVE_OPEN_PLATFORM_GUARD" {
			countKey, unitKey = "guard_num", "guard_unit"
		}
		for _, field := range e.Fields {
			switch field.Name {
			case countKey:
				count, _ = strconv.ParseInt(field.Value, 10, 64)
			case unitKey:
				unit = field.Value
			}
		}
	}
	unit = strings.TrimSpace(unit)
	switch unit {
	case "", "月", "个月":
		if count > 0 {
			return count, "月"
		}
	case "年", "天":
		if count > 0 {
			return count, unit
		}
	default:
		unit = strings.TrimLeft(unit, "*×")
		for _, suffix := range [...]string{"个月", "月", "年", "天"} {
			if value, ok := strings.CutSuffix(unit, suffix); ok {
				if n, err := strconv.ParseInt(value, 10, 64); err == nil && n > 0 {
					return n, strings.TrimPrefix(suffix, "个")
				}
			}
		}
		return 0, unit
	}
	return 0, ""
}

// EventField 保留具名的已解码协议值，不指定 UI 语言。
type EventField struct {
	Name, Value string
}

type projection struct {
	event          Event
	identity, scID string
	deletes        []string
	batch          []projection
}

func object(v any) map[string]any { m, _ := v.(map[string]any); return m }
func array(v any) []any           { a, _ := v.([]any); return a }
func at(v any, n int) any {
	a := array(v)
	if n < len(a) {
		return a[n]
	}
	return nil
}
func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return string(x)
	}
	return ""
}
func number(v any) int64 { n, _ := strconv.ParseInt(stringValue(v), 10, 64); return n }
func strong(v any) string {
	s := stringValue(v)
	if s == "0" {
		return ""
	}
	return s
}
func identity(parts ...string) string { b, _ := json.Marshal(parts); return string(b) }

// decodeEventJSON 保留整数精度，且只接受恰好一个 JSON 值。
func decodeEventJSON(reader io.Reader, out any) bool {
	decoder := json.NewDecoder(reader)
	decoder.UseNumber()
	if decoder.Decode(out) != nil {
		return false
	}
	var extra any
	return decoder.Decode(&extra) == io.EOF
}

func projectMany(room int64, raw []byte) []projection {
	p := project(room, raw)
	if len(p.batch) != 0 {
		return p.batch
	}
	return []projection{p}
}
func project(room int64, raw []byte) projection {
	p := projection{event: Event{RoomID: room, Kind: "unknown", Time: time.Now().UTC()}}
	var root map[string]any
	if !decodeEventJSON(bytes.NewReader(raw), &root) {
		return p
	}
	cmd := strings.SplitN(stringValue(root["cmd"]), ":", 2)[0]
	p.event.Text = cmd
	data := object(root["data"])
	switch cmd {
	case "SEND_GIFT_V2":
		p.batch = projectGiftV2(p.event, stringValue(data["pb"]))
	case "DANMU_MSG", "DANMU_MSG_MIRROR":
		info := root["info"]
		text, ok := at(info, 1).(string)
		if !ok {
			return p
		}
		p.event.Kind, p.event.Text = "chat", text
		p.event.UID, p.event.User = stringValue(at(at(info, 2), 0)), stringValue(at(at(info, 2), 1))
		p.event.MedalLevel, p.event.MedalName = number(at(at(info, 3), 0)), stringValue(at(at(info, 3), 1))
		applySpeakerInfo(&p.event, object(at(at(info, 0), 15)))
		extra := object(at(at(info, 0), 15))["extra"]
		if s, ok := extra.(string); ok {
			var e map[string]any
			dec := json.NewDecoder(strings.NewReader(s))
			dec.UseNumber()
			if dec.Decode(&e) == nil {
				extra = e
			}
		}
		applySpeakerInfo(&p.event, object(extra))
		// rnd 有时只是时间戳：只有明确的服务器消息 ID 才是可靠标识。
		if id := strong(object(extra)["id_str"]); id != "" {
			p.identity = identity("chat", id)
		}
	case "SEND_GIFT":
		if _, ok := data["giftName"].(string); !ok || number(data["num"]) <= 0 {
			return p
		}
		p.event.Kind, p.event.Gift = "gift", stringValue(data["giftName"])
		p.event.User, p.event.UID = stringValue(data["uname"]), stringValue(data["uid"])
		p.event.Count, p.event.Amount, p.event.CoinType = number(data["num"]), number(data["total_coin"]), stringValue(data["coin_type"])
		applySpeakerInfo(&p.event, data)
		// 仅凭批次 ID 无法标识事件。必须有交易标识，并包含
		// 增量和累计数量，以保留同一批次内的更新。
		if tid := strong(data["tid"]); tid != "" {
			batch := object(data["batch_combo_send"])
			p.identity = identity("gift", tid, stringValue(data["rnd"]), p.event.UID, stringValue(data["giftId"]), stringValue(data["timestamp"]), stringValue(data["num"]), stringValue(data["total_coin"]), stringValue(data["combo_num"]), stringValue(batch["batch_combo_num"]), stringValue(batch["combo_num"]), stringValue(batch["total_num"]))
		}
	case "SUPER_CHAT_MESSAGE", "SUPER_CHAT_MESSAGE_JPN":
		if _, ok := data["message"].(string); !ok {
			return p
		}
		p.event.Kind, p.event.Text = "sc", stringValue(data["message"])
		p.event.User, p.event.UID = stringValue(object(data["user_info"])["uname"]), stringValue(data["uid"])
		p.event.Amount = number(data["price"])
		applySpeakerInfo(&p.event, object(data["user_info"]))
		applySpeakerInfo(&p.event, data)
		p.scID = strong(data["id"])
		if p.scID != "" {
			p.identity = identity("sc", p.scID)
		}
	case "SUPER_CHAT_MESSAGE_DELETE":
		if _, ok := data["ids"].([]any); !ok {
			return p
		}
		p.event.Kind = "delete"
		for _, v := range array(data["ids"]) {
			if id := strong(v); id != "" {
				p.deletes = append(p.deletes, id)
			}
		}
	case "GUARD_BUY":
		if data == nil || number(data["num"]) <= 0 {
			return p
		}
		p.event.Kind, p.event.User, p.event.UID = "guard", stringValue(data["username"]), stringValue(data["uid"])
		p.event.Gift, p.event.Count = stringValue(data["gift_name"]), number(data["num"])
		p.event.GuardUnit = "月"
		applySpeakerInfo(&p.event, data)
		// 文档未提供交易 ID：保留无法确定是否重复的上舰记录。
	case "USER_TOAST_MSG_V2", "USER_TOAST_V2":
		if data == nil {
			return p
		}
		sender, guard, pay := object(data["sender_uinfo"]), object(data["guard_info"]), object(data["pay_info"])
		if sender == nil || guard == nil || pay == nil {
			return p
		}
		p.event.Kind, p.event.User, p.event.UID = "guard", stringValue(object(sender["base"])["name"]), stringValue(sender["uid"])
		p.event.Count, p.event.Text = number(pay["num"]), stringValue(data["toast_msg"])
		p.event.Gift = guardName(number(guard["guard_level"]))
		p.event.GuardUnit = stringValue(pay["unit"])
		applySpeakerInfo(&p.event, data)
	default:
		p = projectRoomEvent(p, cmd, root)
		if p.event.Kind == "unknown" {
			p = projectInteractionEvent(p, cmd, root)
		}
		if p.event.Kind == "unknown" {
			p = projectAdditionalCoreEvent(p, cmd, root)
		}
		if p.event.Kind == "unknown" {
			p = projectRankEvent(p, cmd, root)
		}
		if p.event.Kind == "unknown" {
			p = projectCatalogEvent(p, cmd, root)
		}
	}
	return p
}
func guardName(level int64) string {
	switch level {
	case 1:
		return "总督"
	case 2:
		return "提督"
	case 3:
		return "舰长"
	}
	return ""
}

// applySpeakerInfo 只读取发送者实际携带的头像、佩戴粉丝牌和匿名标志。
// 粉丝牌所属直播间不必是当前直播间，舰长等级也不能代替粉丝牌。
func applySpeakerInfo(event *Event, data map[string]any) {
	if face := stringValue(data["face"]); face != "" {
		event.Face = face
	}
	if mystery, _ := data["is_mystery"].(bool); mystery || number(data["is_mystery"]) != 0 {
		event.Mystery = true
	}
	for _, key := range [...]string{"medal_info", "fans_medal"} {
		medal := object(data[key])
		if name := stringValue(medal["medal_name"]); name != "" {
			event.MedalName, event.MedalLevel = name, number(medal["medal_level"])
		}
	}
	for _, key := range [...]string{"user", "uinfo", "sender_uinfo"} {
		user := object(data[key])
		if user == nil {
			continue
		}
		base := object(user["base"])
		if face := stringValue(base["face"]); face != "" {
			event.Face = face
		}
		if mystery, _ := base["is_mystery"].(bool); mystery || number(base["is_mystery"]) != 0 {
			event.Mystery = true
		}
		medal := object(user["medal"])
		if name := stringValue(medal["name"]); name != "" {
			event.MedalName, event.MedalLevel = name, number(medal["level"])
		}
	}
}
