package danmaku

import (
	"encoding/base64"
	"math"
	"strconv"
	"unicode/utf8"

	"google.golang.org/protobuf/encoding/protowire"
)

// Field numbers follow blivedm dev blivedm/models/pb.py's SendGiftBroadcast
// and SendGiftV2GiftItem (https://github.com/xfgryujk/blivedm/blob/dev/blivedm/models/pb.py).
// Unknown fields are skipped using the protobuf wire decoder, not guessed.
func wireFields(raw []byte, field func(protowire.Number, protowire.Type, []byte, uint64) bool) bool {
	for len(raw) != 0 {
		num, typ, n := protowire.ConsumeTag(raw)
		if n < 0 {
			return false
		}
		raw = raw[n:]
		var data []byte
		var value uint64
		switch typ {
		case protowire.BytesType:
			data, n = protowire.ConsumeBytes(raw)
		case protowire.VarintType:
			value, n = protowire.ConsumeVarint(raw)
		default:
			n = protowire.ConsumeFieldValue(num, typ, raw)
		}
		if n < 0 || !field(num, typ, data, value) {
			return false
		}
		raw = raw[n:]
	}
	return true
}
func projectGiftV2(base Event, encoded string) []projection {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil
	}
	var uid, user string
	var items [][]byte
	valid := wireFields(raw, func(num protowire.Number, typ protowire.Type, data []byte, value uint64) bool {
		switch num {
		case 1:
			if typ != protowire.VarintType {
				return false
			}
			uid = strconv.FormatUint(value, 10)
		case 2:
			if typ != protowire.BytesType || !utf8.Valid(data) {
				return false
			}
			user = string(data)
		case 10:
			if typ != protowire.BytesType {
				return false
			}
			items = append(items, data)
		}
		return true
	})
	if !valid || len(items) == 0 {
		return nil
	}
	result := make([]projection, 0, len(items))
	for _, item := range items {
		p := projection{event: base}
		p.event.Kind, p.event.UID, p.event.User = "gift", uid, user
		var giftID, tid, timestamp, rnd string
		valid = wireFields(item, func(num protowire.Number, typ protowire.Type, data []byte, value uint64) bool {
			switch num {
			case 1, 3, 7, 10:
				if typ != protowire.VarintType || value > math.MaxInt64 {
					return false
				}
				switch num {
				case 1:
					giftID = strconv.FormatUint(value, 10)
				case 3:
					p.event.Count = int64(value)
				case 7:
					p.event.Amount = int64(value)
				case 10:
					timestamp = strconv.FormatUint(value, 10)
				}
			case 2, 8, 9, 12:
				if typ != protowire.BytesType || !utf8.Valid(data) {
					return false
				}
				switch num {
				case 2:
					p.event.Gift = string(data)
				case 8:
					p.event.CoinType = string(data)
				case 9:
					tid = string(data)
				case 12:
					rnd = string(data)
				}
			}
			return true
		})
		// An invalid batch remains one raw unknown record: do not silently omit bad items.
		if !valid || p.event.Count <= 0 {
			return nil
		}
		if tid != "" && tid != "0" {
			p.identity = identity("gift", tid, rnd, uid, giftID, timestamp, strconv.FormatInt(p.event.Count, 10), strconv.FormatInt(p.event.Amount, 10), "", "", "", "")
		}
		result = append(result, p)
	}
	return result
}
