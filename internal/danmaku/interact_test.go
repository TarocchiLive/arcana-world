package danmaku

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
)

func interactionV2Fixture(raw []byte) []byte {
	encoded, _ := json.Marshal(map[string]any{"cmd": "INTERACT_WORD_V2", "data": map[string]string{"pb": base64.StdEncoding.EncodeToString(raw)}})
	return encoded
}

func TestInteractionKnownKinds(t *testing.T) {
	for _, tc := range []struct {
		msgType uint64
		kind    string
	}{
		{1, "enter"},
		{2, "follow"},
		{3, "share"},
		{4, "special_follow"},
		{5, "mutual_follow"},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			// 超出 float64 的整数精度；显示的 UID 必须保持精确。
			const uid = uint64(math.MaxUint64)
			pb := protoNumber(nil, 1, uid)
			pb = protoString(pb, 2, "visitor")
			pb = protoNumber(pb, 5, tc.msgType)
			pb = protoString(pb, 99, "future field")
			payloads := [][]byte{interactionV2Fixture(pb)}
			legacy, _ := json.Marshal(map[string]any{"cmd": "INTERACT_WORD", "data": map[string]any{"uid": uid, "uname": "visitor", "msg_type": tc.msgType}})
			payloads = append(payloads, legacy)
			for _, raw := range payloads {
				e := project(7, raw).event
				if e.Kind != tc.kind || e.User != "visitor" || e.UID != "18446744073709551615" || e.Text != "" {
					t.Fatalf("interaction projection: %+v", e)
				}
			}
		})
	}
}

func TestInteractionMalformedAndFutureStayUnknown(t *testing.T) {
	valid := protoNumber(protoString(protoNumber(nil, 1, 42), 2, "visitor"), 5, 1)
	for _, tc := range []struct {
		name string
		raw  []byte
	}{
		{"future enum", interactionV2Fixture(protoNumber(nil, 5, 99))},
		{"missing enum", interactionV2Fixture(protoNumber(nil, 1, 42))},
		{"wrong enum wire type", interactionV2Fixture(protoString(valid, 5, "1"))},
		{"wrong uid wire type", interactionV2Fixture(protoString(valid, 1, "42"))},
		{"invalid utf8", interactionV2Fixture(protoString(valid, 2, string([]byte{0xff})))},
		{"uid overflow", interactionV2Fixture(append(protowire.AppendTag(valid, 1, protowire.VarintType), 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 2))},
		{"enum overflow", interactionV2Fixture(protoNumber(valid, 5, 1<<32|1))},
		{"truncated unknown field", interactionV2Fixture(append(protowire.AppendTag(valid, 99, protowire.BytesType), 0x80))},
		{"invalid base64", []byte(`{"cmd":"INTERACT_WORD_V2","data":{"pb":"!"}}`)},
		{"legacy future enum", []byte(`{"cmd":"INTERACT_WORD","data":{"uid":42,"uname":"visitor","msg_type":99}}`)},
		{"legacy wrong enum type", []byte(`{"cmd":"INTERACT_WORD","data":{"uid":42,"uname":"visitor","msg_type":"1"}}`)},
		{"legacy uid overflow", []byte(`{"cmd":"INTERACT_WORD","data":{"uid":"18446744073709551616","uname":"visitor","msg_type":1}}`)},
		{"legacy wrong user type", []byte(`{"cmd":"INTERACT_WORD","data":{"uid":42,"uname":12,"msg_type":1}}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := project(7, tc.raw).event
			var root map[string]any
			if err := json.Unmarshal(tc.raw, &root); err != nil {
				t.Fatal(err)
			}
			if e.Kind != "unknown" || e.Text != root["cmd"] || e.User != "" || e.UID != "" {
				t.Fatalf("invalid interaction was projected: %+v", e)
			}
		})
	}
}

func TestInteractionNestedUserFallback(t *testing.T) {
	uinfo := protoNumber(nil, 1, math.MaxUint64)
	uinfo = protoString(uinfo, 2, string(protoString(nil, 1, "nested visitor")))
	pb := protoString(protoNumber(nil, 5, 4), 22, string(uinfo))
	e := project(7, interactionV2Fixture(pb)).event
	if e.Kind != "special_follow" || e.UID != "18446744073709551615" || e.User != "nested visitor" {
		t.Fatalf("nested user lost: %+v", e)
	}
	pb = protoString(protoNumber(pb, 1, 42), 2, "primary visitor")
	e = project(7, interactionV2Fixture(pb)).event
	if e.UID != "42" || e.User != "primary visitor" {
		t.Fatalf("primary user did not take precedence: %+v", e)
	}
	bad := protoString(protoNumber(nil, 5, 1), 22, string(protoNumber(nil, 2, 1)))
	if e := project(7, interactionV2Fixture(bad)).event; e.Kind != "unknown" {
		t.Fatalf("malformed nested user accepted: %+v", e)
	}
}
