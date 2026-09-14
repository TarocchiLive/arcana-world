package danmaku

import "testing"

func TestRoomCountersDistinguishZeroMissingAndInvalid(t *testing.T) {
	for _, raw := range []string{
		`{"cmd":"WATCHED_CHANGE","data":{}}`,
		`{"cmd":"WATCHED_CHANGE","data":{"num":"0"}}`,
		`{"cmd":"LIKE_INFO_V3_UPDATE","data":{"click_count":1.5}}`,
		`{"cmd":"ONLINE_RANK_COUNT","data":{"count":12,"online_count":null}}`,
		`{"cmd":"WATCHED_CHANGE","data":{"num":9223372036854775808}}`,
		`{"cmd":"WATCHED_CHANGE","data":{"num":-1}}`,
	} {
		if event := project(1, []byte(raw)).event; event.Kind != "unknown" {
			t.Fatalf("invalid counter became a statistic: %+v", event)
		}
	}
	if event := project(1, []byte(`{"cmd":"WATCHED_CHANGE","data":{"num":0}}`)).event; event.Kind != "watched" || event.Count != 0 {
		t.Fatalf("explicit zero was lost: %+v", event)
	}
	if event := project(1, []byte(`{"cmd":"ONLINE_RANK_COUNT","data":{"count":12,"online_count":34}}`)).event; event.Kind != "rank_count" || event.Count != 34 {
		t.Fatalf("new rank count did not take precedence: %+v", event)
	}
}

func TestStopRoomListRejectsPartialPayload(t *testing.T) {
	if event := project(7, []byte(`{"cmd":"STOP_LIVE_ROOM_LIST","data":{"room_id_list":[7,null]}}`)).event; event.Kind != "unknown" {
		t.Fatalf("partial list became a valid stop event: %+v", event)
	}
	if event := project(7, []byte(`{"cmd":"STOP_LIVE_ROOM_LIST","data":{"room_id_list":["7",8]}}`)).event; event.Kind != "stop_rooms" || event.Count != 2 || event.Amount != 1 {
		t.Fatalf("decimal room ID membership was lost: %+v", event)
	}
}

func TestNoticeRetainsTopLevelBodyAndSegmentOrder(t *testing.T) {
	if event := project(1, []byte(`{"cmd":"NOTICE_MSG","msg_common":"public notice","msg_self":"local notice","data":{"msg_common":"not the notice"}}`)).event; event.Kind != "notice" || event.Text != "public notice" {
		t.Fatalf("top-level notice body was replaced: %+v", event)
	}
	if event := project(1, []byte(`{"cmd":"LIKE_INFO_V3_NOTICE","data":{"content_segments":[{"type":1,"text":"first "},{"type":987,"text":"second"}]}}`)).event; event.Kind != "notice" || event.Text != "first second" {
		t.Fatalf("server text was reordered or interpreted as an enum: %+v", event)
	}
	if event := project(1, []byte(`{"cmd":"LIKE_INFO_V3_NOTICE","data":{"content_segments":[{"text":"first"},{"text":3}]}}`)).event; event.Kind != "unknown" {
		t.Fatalf("partial notification was displayed: %+v", event)
	}
}
