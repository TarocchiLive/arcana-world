// Package danmaku retains received business events and conservative projections.
package danmaku

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

// Event is the display projection; raw wire data is stored separately.
// Time is the local receive time. Gift Amount is raw CoinType units; SC is yuan.
type Event struct {
	Sequence                              uint64
	RoomID                                int64
	Kind, User, UID, Text, Gift, CoinType string
	Count                                 int64
	Amount                                int64
	Time                                  time.Time
	Deleted                               bool
	Title                                 string       `json:",omitempty"`
	Fields                                []EventField `json:",omitempty"`
}

// EventField retains a named, decoded protocol value without selecting a UI language.
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
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if !json.Valid(raw) || d.Decode(&root) != nil {
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
		extra := object(at(at(info, 0), 15))["extra"]
		if s, ok := extra.(string); ok {
			var e map[string]any
			dec := json.NewDecoder(strings.NewReader(s))
			dec.UseNumber()
			if dec.Decode(&e) == nil {
				extra = e
			}
		}
		// rnd is sometimes merely a timestamp: only the explicit server message ID is strong.
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
		// Batch IDs alone are not event identities. Require a transaction and include
		// increment/cumulative quantities so updates within a batch survive.
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
		// No documented transaction ID: retain ambiguous guard representations.
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
