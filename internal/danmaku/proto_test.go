package danmaku

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func protoString(b []byte, field protowire.Number, value string) []byte {
	return protowire.AppendString(protowire.AppendTag(b, field, protowire.BytesType), value)
}
func protoNumber(b []byte, field protowire.Number, value uint64) []byte {
	return protowire.AppendVarint(protowire.AppendTag(b, field, protowire.VarintType), value)
}
func giftV2Fixture(items ...[]byte) string {
	b := protoNumber(nil, 1, 42)
	b = protoString(b, 2, "alice")
	for _, item := range items {
		b = protowire.AppendBytes(protowire.AppendTag(b, 10, protowire.BytesType), item)
	}
	raw, _ := json.Marshal(map[string]any{"cmd": "SEND_GIFT_V2", "data": map[string]string{"pb": base64.StdEncoding.EncodeToString(b)}})
	return string(raw)
}
func giftV2Item(count uint64) []byte {
	b := protoNumber(nil, 1, 1)
	b = protoString(b, 2, "flower")
	b = protoNumber(b, 3, count)
	b = protoNumber(b, 7, count*1000)
	b = protoString(b, 8, "gold")
	b = protoString(b, 9, "transaction")
	b = protoNumber(b, 10, 1700000000)
	b = protoString(b, 12, "rnd")
	return protoString(b, 99, "future field")
}
func TestGiftV2BatchReplayAndLegacyIdentity(t *testing.T) {
	dir := t.TempDir()
	h := openHistory(t, dir)
	raw := giftV2Fixture(giftV2Item(1), giftV2Item(2))
	appendEvent(t, h, 1, raw, true)
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h = openHistory(t, dir)
	appendEvent(t, h, 1, raw, false)
	appendEvent(t, h, 1, `{"cmd":"SEND_GIFT","data":{"tid":"transaction","rnd":"rnd","uid":42,"giftId":1,"giftName":"flower","num":2,"total_coin":2000,"coin_type":"gold","timestamp":1700000000}}`, false)
	events := page(t, h, 1, 0, 10)
	if len(events) != 2 || events[0].Kind != "gift" || events[0].Count != 2 || events[0].Amount != 2000 || events[0].Gift != "flower" || events[0].CoinType != "gold" || events[0].User != "alice" || events[1].Count != 1 {
		t.Fatalf("batch gifts: %+v", events)
	}
}
func TestGiftV2MalformedBatchRetainedWithoutPartialProjection(t *testing.T) {
	h := openHistory(t, t.TempDir())
	// A truncated second item must not make the first look like a complete batch.
	appendEvent(t, h, 1, giftV2Fixture(giftV2Item(1), []byte{0x12, 0x80}), true)
	appendEvent(t, h, 1, `{"cmd":"SEND_GIFT_V2","data":{"pb":"not base64"}}`, true)
	events := page(t, h, 1, 0, 10)
	if len(events) != 2 || events[0].Kind != "unknown" || events[1].Kind != "unknown" {
		t.Fatalf("malformed batches: %+v", events)
	}
}
