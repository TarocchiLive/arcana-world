package danmaku

import (
	"encoding/json"
	"fmt"
	"testing"
)

func openHistory(t *testing.T, dir string) *History {
	t.Helper()
	h, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.Close(); err != nil {
			t.Error(err)
		}
	})
	return h
}
func appendEvent(t *testing.T, h *History, room int64, raw string, want bool) {
	t.Helper()
	got, err := h.Append(room, json.RawMessage(raw))
	if err != nil || got != want {
		t.Fatalf("Append inserted=%v err=%v, want %v", got, err, want)
	}
}
func page(t *testing.T, h *History, room int64, before uint64, limit int) []Event {
	t.Helper()
	events, err := h.Page(room, before, limit)
	if err != nil {
		t.Fatal(err)
	}
	return events
}
func chat(id string) string {
	return fmt.Sprintf(`{"cmd":"DANMU_MSG:4:0:2:2:2:0","info":[[0,1,25,0,1700000000000,1700000000,0,0,0,0,0,0,0,0,0,{"extra":"{\"id_str\":\"%s\"}"}],"hello",[42,"alice"]]}`, id)
}

func TestReplayAcrossReopenAndRoomIsolation(t *testing.T) {
	dir := t.TempDir()
	h := openHistory(t, dir)
	appendEvent(t, h, 1, chat("server-id"), true)
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h = openHistory(t, dir)
	appendEvent(t, h, 1, chat("server-id"), false)
	appendEvent(t, h, 2, chat("server-id"), true)
	for _, room := range []int64{1, 2} {
		events := page(t, h, room, 0, 10)
		if len(events) != 1 || events[0].Text != "hello" || events[0].User != "alice" || events[0].RoomID != room {
			t.Fatalf("room %d: %+v", room, events)
		}
	}
	rooms, err := h.Rooms()
	if err != nil || len(rooms) != 2 || rooms[0] != 1 || rooms[1] != 2 {
		t.Fatalf("rooms=%v err=%v", rooms, err)
	}
}
func TestSuperChatDeletionBothOrdersAndReopen(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprint(reverse), func(t *testing.T) {
			dir := t.TempDir()
			h := openHistory(t, dir)
			sc := `{"cmd":"SUPER_CHAT_MESSAGE","data":{"id":9007199254740993,"uid":42,"message":"private text","price":30,"user_info":{"uname":"alice"}}}`
			del := `{"cmd":"SUPER_CHAT_MESSAGE_DELETE","data":{"ids":[9007199254740993]}}`
			if reverse {
				appendEvent(t, h, 1, del, true)
				if err := h.Close(); err != nil {
					t.Fatal(err)
				}
				h = openHistory(t, dir)
				appendEvent(t, h, 1, sc, true)
			} else {
				appendEvent(t, h, 1, sc, true)
				appendEvent(t, h, 1, del, true)
			}
			if err := h.Close(); err != nil {
				t.Fatal(err)
			}
			h = openHistory(t, dir)
			appendEvent(t, h, 1, sc, false)
			events := page(t, h, 1, 0, 10)
			if len(events) != 2 {
				t.Fatalf("records: %+v", events)
			}
			for _, e := range events {
				if e.Kind == "sc" && (!e.Deleted || e.Text != "" || e.Amount != 30) {
					t.Fatalf("SC resurrected: %+v", e)
				}
			}
			appendEvent(t, h, 2, sc, true)
			if e := page(t, h, 2, 0, 1)[0]; e.Deleted || e.Text != "private text" {
				t.Fatalf("cross-room tombstone: %+v", e)
			}
		})
	}
}
