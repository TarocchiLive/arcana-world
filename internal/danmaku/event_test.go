package danmaku

import "testing"

func TestGuardRepresentationsRemainDistinctWithoutTransactionID(t *testing.T) {
	h := openHistory(t, t.TempDir())
	appendEvent(t, h, 1, `{"cmd":"GUARD_BUY","data":{"uid":42,"username":"alice","guard_level":3,"num":1,"price":198000,"gift_name":"舰长","start_time":1700000000}}`, true)
	toast := `{"cmd":"USER_TOAST_MSG_V2","data":{"sender_uinfo":{"uid":42,"base":{"name":"alice"}},"guard_info":{"guard_level":3,"start_time":1700000000},"pay_info":{"num":1,"price":198000,"unit":"月"},"gift_info":{"gift_id":10003},"option":{"source":0},"toast_msg":"开通舰长"}}`
	appendEvent(t, h, 1, toast, true)
	// 时间戳、用户和数量相同，并不能证明是同一次购买。
	appendEvent(t, h, 1, toast, true)
	events := page(t, h, 1, 0, 10)
	if len(events) != 3 {
		t.Fatalf("ambiguous guard purchases lost: %+v", events)
	}
	for _, event := range events {
		if event.Kind != "guard" || event.User != "alice" || event.UID != "42" || event.Gift != "舰长" || event.Count != 1 {
			t.Fatalf("guard projection: %+v", event)
		}
	}
}

func TestMalformedKnownCommandsRemainUnknown(t *testing.T) {
	for _, raw := range []string{
		`{"cmd":"SEND_GIFT","data":{}}`,
		`{"cmd":"SUPER_CHAT_MESSAGE","data":{"message":42}}`,
		`{"cmd":"SUPER_CHAT_MESSAGE_DELETE","data":{"ids":42}}`,
		`{"cmd":"GUARD_BUY","data":{}}`,
		`{"cmd":"USER_TOAST_MSG_V2","data":{"sender_uinfo":42}}`,
	} {
		if event := project(1, []byte(raw)).event; event.Kind != "unknown" {
			t.Fatalf("malformed projection: %+v", event)
		}
	}
}

func TestSpeakerMetadataSchemas(t *testing.T) {
	for _, raw := range []string{
		`{"cmd":"DANMU_MSG","info":[[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,{"extra":"{\"user\":{\"base\":{\"face\":\"avatar\",\"is_mystery\":true},\"medal\":{\"name\":\"worn\",\"level\":12}}}"}],"hello",[42,"alice"],[]]}`,
		`{"cmd":"SUPER_CHAT_MESSAGE","data":{"uid":42,"message":"hello","user_info":{"uname":"alice","face":"avatar"},"medal_info":{"medal_name":"worn","medal_level":12,"anchor_roomid":999},"is_mystery":1}}`,
		`{"cmd":"SEND_GIFT","data":{"uid":42,"uname":"alice","giftName":"gift","num":1,"face":"avatar","medal_info":{"medal_name":"worn","medal_level":12},"is_mystery":true}}`,
	} {
		event := project(1, []byte(raw)).event
		if event.Face != "avatar" || event.MedalName != "worn" || event.MedalLevel != 12 || !event.Mystery || event.UID != "42" {
			t.Fatalf("speaker schema lost: %+v", event)
		}
	}
}
