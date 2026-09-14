package danmaku

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"strconv"
	"unicode/utf8"

	"arcana-world/internal/i18n"
	"google.golang.org/protobuf/encoding/protowire"
)

// ONLINE_RANK_V3 decodes typed protobuf fields rather than displaying opaque bytes.
// UID strings preserve the schema's full uint64 range in JSON and in the UI.
type rankProtoField struct {
	number   protowire.Number
	name     string
	kind     byte
	children []rankProtoField
}

var rankProtoUserFields = []rankProtoField{
	{1, "uid", 'i', nil},
	{2, "base", 'o', []rankProtoField{
		{1, "name", 's', nil}, {2, "face", 's', nil},
		{3, "name_color", 'n', nil}, {4, "is_mystery", 'b', nil},
	}},
	{6, "guard", 'o', []rankProtoField{
		{1, "level", 'n', nil}, {2, "expired_str", 's', nil},
	}},
}

var rankProtoEntryFields = []rankProtoField{
	{1, "uid", 'i', nil}, {2, "face", 's', nil},
	{3, "score", 's', nil}, {4, "uname", 's', nil},
	{5, "rank", 'n', nil}, {6, "guard_level", 'n', nil},
	{7, "is_mystery", 'b', nil}, {8, "uinfo", 'o', rankProtoUserFields},
}

func decodeRankProtoObject(raw []byte, fields []rankProtoField) (map[string]any, bool) {
	result := make(map[string]any)
	valid := wireFields(raw, func(num protowire.Number, typ protowire.Type, data []byte, value uint64) bool {
		for _, field := range fields {
			if field.number != num {
				continue
			}
			switch field.kind {
			case 's':
				if typ != protowire.BytesType || !utf8.Valid(data) {
					return false
				}
				result[field.name] = string(data)
			case 'i', 'n', 'b':
				if typ != protowire.VarintType {
					return false
				}
				switch field.kind {
				case 'i':
					result[field.name] = strconv.FormatUint(value, 10)
				case 'n':
					if value > math.MaxUint32 {
						return false
					}
					result[field.name] = value
				case 'b':
					if value > 1 {
						return false
					}
					result[field.name] = value == 1
				}
			case 'o':
				if typ != protowire.BytesType {
					return false
				}
				nested, ok := decodeRankProtoObject(data, field.children)
				if !ok {
					return false
				}
				result[field.name] = nested
			}
			return true
		}
		return true
	})
	return result, valid
}

func projectRankEvent(p projection, cmd string, root map[string]any) projection {
	if cmd != "ONLINE_RANK_V3" {
		return p
	}
	encoded, ok := object(root["data"])["pb"].(string)
	if !ok {
		return p
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return p
	}
	var rankType string
	var recognized bool
	entries := make([]map[string]any, 0)
	valid := wireFields(raw, func(num protowire.Number, typ protowire.Type, data []byte, _ uint64) bool {
		switch num {
		case 1:
			if typ != protowire.BytesType || !utf8.Valid(data) {
				return false
			}
			rankType, recognized = string(data), true
		case 3:
			if typ != protowire.BytesType {
				return false
			}
			entry, ok := decodeRankProtoObject(data, rankProtoEntryFields)
			if !ok {
				return false
			}
			// Required proto3 scalars omitted on the wire have their zero value.
			for _, field := range rankProtoEntryFields[:5] {
				if _, exists := entry[field.name]; !exists {
					if field.kind == 'n' {
						entry[field.name] = uint64(0)
					} else if field.kind == 'i' {
						entry[field.name] = "0"
					} else {
						entry[field.name] = ""
					}
				}
			}
			entries, recognized = append(entries, entry), true
		}
		return true
	})
	if !valid || !recognized {
		return p
	}
	list, err := json.Marshal(entries)
	if err != nil {
		return p
	}
	p.event.Kind, p.event.Title, p.event.Text = "detail", string(i18n.DanmakuOnlineRankV3), cmd
	p.event.Fields = []EventField{{Name: "data.rank_type", Value: rankType}, {Name: "data.online_list", Value: string(list)}}
	return p
}
