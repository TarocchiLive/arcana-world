package danmaku

import (
	"strings"
	"testing"
)

func TestCatalogEmbeddedJSONPreservesIntegerPrecision(t *testing.T) {
	p := project(1, []byte(`{"cmd":"DM_INTERACTION","data":{"data":"{\"combo\":[{\"uid\":18446744073709551615}],\"future\":987}"}}`))
	if p.event.Kind != "detail" {
		t.Fatalf("embedded object was not decoded: %+v", p.event)
	}
	for _, field := range p.event.Fields {
		if field.Name == "data" && strings.Contains(field.Value, `"uid":18446744073709551615`) && strings.Contains(field.Value, `"future":987`) {
			return
		}
	}
	t.Fatal("nested identifier or future enum lost precision or content")
}

func TestCatalogRejectsInvalidShapesWithoutGuessingFutureCommands(t *testing.T) {
	for _, raw := range []string{
		`{"cmd":"DM_INTERACTION","data":{"data":"{\"combo\":[]} null"}}`,
		`{"cmd":"DM_INTERACTION","data":{"data":true}}`,
		`{"cmd":"ENTRY_EFFECT","data":{"uid":"18446744073709551616"}}`,
		`{"cmd":"ENTRY_EFFECT","data":{"uid":42,"copy_writing":false}}`,
		`{"cmd":"PK_BATTLE_UNPUBLISHED","data":{"id":42,"msg":"not a known command"}}`,
	} {
		if p := project(1, []byte(raw)); p.event.Kind != "unknown" {
			t.Fatalf("invalid or unpublished event claimed as parsed: %s", raw)
		}
	}
}

func TestOpenPlatformSCDeletionDoesNotEraseWebSCWithSameID(t *testing.T) {
	h := openHistory(t, t.TempDir())
	appendEvent(t, h, 1, `{"cmd":"SUPER_CHAT_MESSAGE","data":{"id":42,"message":"web","price":30,"user_info":{"uname":"viewer"}}}`, true)
	open := `{"cmd":"LIVE_OPEN_PLATFORM_SUPER_CHAT","data":{"message_id":42,"message":"open","rmb":30,"uname":"viewer","open_id":"opaque-user"}}`
	appendEvent(t, h, 1, open, true)
	appendEvent(t, h, 1, open, false)
	appendEvent(t, h, 1, `{"cmd":"LIVE_OPEN_PLATFORM_SUPER_CHAT_DEL","data":{"message_ids":[42]}}`, true)
	events := page(t, h, 1, 0, 10)
	if len(events) != 3 || !events[1].Deleted || events[1].Text != "" || events[2].Deleted || events[2].Text != "web" {
		t.Fatalf("SC deletion crossed protocol identity boundaries: %+v", events)
	}
}
