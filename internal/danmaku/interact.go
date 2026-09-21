package danmaku

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"strconv"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

// 旧版 JSON 和 V2 protobuf 互动共用动作值 1–5。
func projectInteractionEvent(p projection, cmd string, root map[string]any) projection {
	data := object(root["data"])
	var uid, user string
	var msgType uint64
	switch cmd {
	case "INTERACT_WORD":
		typ, ok := data["msg_type"].(json.Number)
		if !ok {
			return p
		}
		var err error
		msgType, err = strconv.ParseUint(string(typ), 10, 32)
		if err != nil {
			return p
		}
		if value, exists := data["uid"]; exists {
			id, err := strconv.ParseUint(stringValue(value), 10, 64)
			if err != nil {
				return p
			}
			uid = strconv.FormatUint(id, 10)
		}
		if value, exists := data["uname"]; exists {
			user, ok = value.(string)
			if !ok || !utf8.ValidString(user) {
				return p
			}
		}
	case "INTERACT_WORD_V2":
		encoded, ok := data["pb"].(string)
		if !ok {
			return p
		}
		raw, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			return p
		}
		var nestedUID, nestedUser string
		var hasUID, hasUser bool
		valid := wireFields(raw, func(num protowire.Number, typ protowire.Type, bytes []byte, value uint64) bool {
			switch num {
			case 1:
				if typ != protowire.VarintType {
					return false
				}
				uid = strconv.FormatUint(value, 10)
				hasUID = true
			case 2:
				if typ != protowire.BytesType || !utf8.Valid(bytes) {
					return false
				}
				user = string(bytes)
				hasUser = true
			case 5:
				if typ != protowire.VarintType || value > math.MaxUint32 {
					return false
				}
				msgType = value
			case 22:
				if typ != protowire.BytesType {
					return false
				}
				var ok bool
				nestedUID, nestedUser, ok = decodeInteractionUser(bytes)
				if !ok {
					return false
				}
			}
			return true
		})
		if !valid {
			return p
		}
		if !hasUID {
			uid = nestedUID
		}
		if !hasUser {
			user = nestedUser
		}
	default:
		return p
	}
	var kind string
	switch msgType {
	case 1:
		kind = "enter"
	case 2:
		kind = "follow"
	case 3:
		kind = "share"
	case 4:
		kind = "special_follow"
	case 5:
		kind = "mutual_follow"
	default:
		return p
	}
	p.event.Kind, p.event.User, p.event.UID, p.event.Text = kind, user, uid, ""
	return p
}

func decodeInteractionUser(raw []byte) (uid, user string, valid bool) {
	valid = wireFields(raw, func(num protowire.Number, typ protowire.Type, data []byte, value uint64) bool {
		switch num {
		case 1:
			if typ != protowire.VarintType {
				return false
			}
			uid = strconv.FormatUint(value, 10)
		case 2:
			if typ != protowire.BytesType {
				return false
			}
			return wireFields(data, func(num protowire.Number, typ protowire.Type, data []byte, _ uint64) bool {
				switch num {
				case 1, 2, 3:
					if typ != protowire.BytesType || !utf8.Valid(data) {
						return false
					}
					if num == 1 {
						user = string(data)
					}
				}
				return true
			})
		}
		return true
	})
	return
}
