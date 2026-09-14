package danmaku

import (
	"encoding/base64"
	"encoding/json"
	"math"
	"testing"

	"arcana-world/internal/i18n"
	"google.golang.org/protobuf/encoding/protowire"
)

func rankProjectionFixture(raw []byte) Event {
	return projectRankEvent(projection{event: Event{Kind: "unknown", Text: "ONLINE_RANK_V3"}}, "ONLINE_RANK_V3", map[string]any{
		"data": map[string]any{"pb": base64.StdEncoding.EncodeToString(raw)},
	}).event
}

func TestRankProtoListAndPrecision(t *testing.T) {
	first := protoNumber(nil, 1, math.MaxUint64)
	first = protoString(first, 3, "999999999999999999999")
	first = protoString(first, 4, "first viewer")
	first = protoNumber(first, 5, 1)
	first = protoNumber(first, 6, 99) // Future guard enums retain their raw value.
	first = protoString(first, 8, string(protoString(nil, 2, string(protoString(nil, 1, "profile name")))))
	second := protoNumber(protoString(protoNumber(nil, 1, 42), 4, "second viewer"), 5, 2)
	pb := protoString(protoString(protoString(nil, 1, "online_rank"), 3, string(first)), 3, string(second))
	pb = protoString(pb, 99, "future field")
	e := rankProjectionFixture(pb)
	if e.Kind != "detail" || e.Title != string(i18n.DanmakuOnlineRankV3) || e.Text != "ONLINE_RANK_V3" {
		t.Fatalf("rank projection: %+v", e)
	}
	var entries []struct {
		UID   string `json:"uid"`
		Score string `json:"score"`
		Name  string `json:"uname"`
		Rank  uint32 `json:"rank"`
		Guard uint32 `json:"guard_level"`
		UInfo struct {
			Base struct {
				Name string `json:"name"`
			} `json:"base"`
		} `json:"uinfo"`
	}
	for _, field := range e.Fields {
		if field.Name == "data.online_list" {
			if err := json.Unmarshal([]byte(field.Value), &entries); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(entries) != 2 || entries[0].UID != "18446744073709551615" || entries[0].Score != "999999999999999999999" || entries[0].Name != "first viewer" || entries[0].Rank != 1 || entries[0].Guard != 99 || entries[0].UInfo.Base.Name != "profile name" || entries[1].UID != "42" || entries[1].Rank != 2 {
		t.Fatalf("rank data lost: %+v", entries)
	}
}

func TestRankProtoMalformedStaysUnknown(t *testing.T) {
	valid := protoString(nil, 1, "online_rank")
	cases := map[string][]byte{
		"empty":               nil,
		"unknown fields only": protoNumber(nil, 99, 1),
		"wrong list type":     protoNumber(valid, 3, 1),
		"wrong uid type":      protoString(valid, 3, string(protoString(nil, 1, "42"))),
		"rank overflow":       protoString(valid, 3, string(protoNumber(nil, 5, 1<<32))),
		"invalid bool":        protoString(valid, 3, string(protoNumber(nil, 7, 2))),
		"invalid utf8":        protoString(valid, 3, string(protoString(nil, 4, string([]byte{0xff})))),
		"wrong nested type":   protoString(valid, 3, string(protoString(nil, 8, string(protoNumber(nil, 2, 1))))),
		"truncated unknown":   append(protowire.AppendTag(valid, 99, protowire.BytesType), 0x80),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if e := rankProjectionFixture(raw); e.Kind != "unknown" || e.Text != "ONLINE_RANK_V3" || len(e.Fields) != 0 {
				t.Fatalf("malformed rank accepted: %+v", e)
			}
		})
	}
	if e := rankProjectionFixture(valid); e.Kind != "detail" || e.Fields[1].Value != "[]" {
		t.Fatalf("empty ranking lost: %+v", e)
	}
}
