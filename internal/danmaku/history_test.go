package danmaku

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
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
func TestAmbiguousRepeatedChatsSurvive(t *testing.T) {
	h := openHistory(t, t.TempDir())
	for range 3 {
		appendEvent(t, h, 1, chat(""), true)
	}
	events := page(t, h, 1, 0, 10)
	if len(events) != 3 {
		t.Fatalf("lost repeated chats: %+v", events)
	}
}
func TestGiftTransactionIncrements(t *testing.T) {
	h := openHistory(t, t.TempDir())
	gift := func(n int) string {
		return fmt.Sprintf(`{"cmd":"SEND_GIFT","data":{"tid":"transaction","rnd":"rnd","batch_combo_id":"batch","uid":42,"giftId":1,"giftName":"flower","num":%d,"total_coin":%d,"coin_type":"gold","timestamp":1700000000}}`, n, n*1000)
	}
	appendEvent(t, h, 1, gift(1), true)
	appendEvent(t, h, 1, gift(2), true)
	appendEvent(t, h, 1, gift(2), false)
	events := page(t, h, 1, 0, 10)
	if len(events) != 2 || events[0].Count != 2 || events[0].Amount != 2000 || events[0].CoinType != "gold" {
		t.Fatalf("gift increments: %+v", events)
	}
}
func TestMalformedAndUnknownRawRetention(t *testing.T) {
	h := openHistory(t, t.TempDir())
	raws := []string{`{broken`, `null`, `{"cmd":"DANMU_MSG","info":[null]}`, `{"cmd":"FUTURE_EVENT","data":{"new":true}}`}
	for _, raw := range raws {
		appendEvent(t, h, 1, raw, true)
	}
	events := page(t, h, 1, 0, 10)
	if len(events) != len(raws) {
		t.Fatalf("lost unknown events: %+v", events)
	}
	for _, e := range events {
		if e.Kind != "unknown" {
			t.Fatalf("unexpected projection: %+v", e)
		}
	}
	if err := h.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(roomsBucket).Bucket(key(1)).Bucket(rawBucket)
		for i, raw := range raws {
			if string(b.Get(key(uint64(i+1)))) != raw {
				return fmt.Errorf("raw record %d changed", i)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
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
func TestPaginationExactCoverageAndGap(t *testing.T) {
	h := openHistory(t, t.TempDir())
	for i := range 7 {
		if err := h.RecordGap(1, fmt.Sprint(i)); err != nil {
			t.Fatal(err)
		}
	}
	var before uint64
	want := uint64(7)
	for {
		events := page(t, h, 1, before, 3)
		if len(events) == 0 {
			break
		}
		for _, e := range events {
			if e.Sequence != want || e.Kind != "gap" || e.Text != fmt.Sprint(want-1) || e.Time.IsZero() {
				t.Fatalf("pagination: got %+v want sequence %d", e, want)
			}
			want--
		}
		before = events[len(events)-1].Sequence
	}
	if want != 0 {
		t.Fatalf("missing %d records", want)
	}
	if len(page(t, h, 1, 1, 10)) != 0 || len(page(t, h, 1, 0, 0)) != 0 || len(page(t, h, 1, 0, -1)) != 0 || len(page(t, h, 999, 0, 10)) != 0 {
		t.Fatal("invalid page boundaries")
	}
	if len(page(t, h, 1, 99, 10)) != 7 {
		t.Fatal("cursor beyond latest omitted events")
	}
}
func TestClosePermissionsAndLock(t *testing.T) {
	dir := t.TempDir()
	h := openHistory(t, dir)
	for path, mode := range map[string]os.FileMode{filepath.Join(dir, "danmaku"): 0700, filepath.Join(dir, "danmaku", "history.db"): 0600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != mode {
			t.Fatalf("unsafe permissions: %v", info.Mode())
		}
	}
	if other, err := Open(dir); err == nil {
		other.Close()
		t.Fatal("second writer acquired locked history")
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if inserted, err := h.Append(1, json.RawMessage(`{}`)); inserted || !errors.Is(err, os.ErrClosed) {
		t.Fatalf("closed append=%v err=%v", inserted, err)
	}
}

func TestRetentionBoundaryRawAndIdentity(t *testing.T) {
	h := openHistory(t, t.TempDir())
	now := time.Now().UTC()
	cutoff := now.Add(-retention)
	for i, at := range []time.Time{cutoff.Add(-time.Nanosecond), cutoff, now} {
		p := project(1, []byte(chat(fmt.Sprint(i))))
		p.event.Time = at
		if _, err := h.append(1, []byte(chat(fmt.Sprint(i))), p); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.prune(now); err != nil {
		t.Fatal(err)
	}
	events := page(t, h, 1, 0, 10)
	if len(events) != 2 || events[0].Sequence != 3 || events[1].Sequence != 2 {
		t.Fatalf("retention boundary: %+v", events)
	}
	if err := h.db.View(func(tx *bolt.Tx) error {
		raw := tx.Bucket(roomsBucket).Bucket(key(1)).Bucket(rawBucket)
		if raw.Get(key(1)) != nil || raw.Get(key(2)) == nil || raw.Get(key(3)) == nil {
			return errors.New("raw retention differs from event retention")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	appendEvent(t, h, 1, chat("0"), true)
	appendEvent(t, h, 1, chat("1"), false)
}

func TestRetentionStartupMigratesAndExpiresTombstones(t *testing.T) {
	dir := t.TempDir()
	h := openHistory(t, dir)
	sc := `{"cmd":"SUPER_CHAT_MESSAGE","data":{"id":17,"message":"fresh","price":30}}`
	del := []byte(`{"cmd":"SUPER_CHAT_MESSAGE_DELETE","data":{"ids":[17]}}`)
	p := project(1, del)
	if _, err := h.append(1, del, p); err != nil {
		t.Fatal(err)
	}
	// Emulate the previous on-disk format, which had no deletion sequence.
	if err := h.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(roomsBucket).Bucket(key(1)).Bucket(tombstonesBucket).Put([]byte("17"), []byte{1})
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h = openHistory(t, dir)
	appendEvent(t, h, 1, sc, true)
	if !page(t, h, 1, 0, 1)[0].Deleted {
		t.Fatal("retained legacy deletion resurrected")
	}
	// Age both the deletion and its SC, then prove startup expires their
	// raw payloads, dedup identity and deletion state together.
	if err := h.db.Update(func(tx *bolt.Tx) error {
		events := tx.Bucket(roomsBucket).Bucket(key(1)).Bucket(eventsBucket)
		return events.ForEach(func(k, v []byte) error {
			var e Event
			if err := json.Unmarshal(v, &e); err != nil {
				return err
			}
			e.Time = time.Now().Add(-retention - time.Hour)
			v, err := json.Marshal(e)
			if err != nil {
				return err
			}
			return events.Put(k, v)
		})
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h = openHistory(t, dir)
	if events := page(t, h, 1, 0, 10); len(events) != 0 {
		t.Fatalf("startup retained expired events: %+v", events)
	}
	appendEvent(t, h, 1, sc, true)
	if e := page(t, h, 1, 0, 1)[0]; e.Deleted || e.Text != "fresh" {
		t.Fatalf("expired tombstone blocked new SC: %+v", e)
	}
}

func TestHistoryReinterpretsUnknownWithoutMovingItsCursor(t *testing.T) {
	h := openHistory(t, t.TempDir())
	received := time.Now().UTC().Add(-time.Hour)
	raw := []byte(`{"cmd":"WATCHED_CHANGE","data":{"num":42}}`)
	if _, err := h.append(1, raw, projection{event: Event{
		RoomID: 1, Kind: "unknown", Text: "WATCHED_CHANGE", Time: received,
	}}); err != nil {
		t.Fatal(err)
	}
	appendEvent(t, h, 1, `{"cmd":"FUTURE_EVENT"}`, true)
	events := page(t, h, 1, 2, 10)
	if len(events) != 1 || events[0].Kind != "watched" || events[0].Count != 42 {
		t.Fatalf("saved unknown event was not decoded: %+v", events)
	}
	if events[0].Sequence != 1 || !events[0].Time.Equal(received) {
		t.Fatal("decoding changed history order or receive time")
	}
	latest := page(t, h, 1, 0, 1)
	if len(latest) != 1 || latest[0].Kind != "unknown" || latest[0].Text != "FUTURE_EVENT" {
		t.Fatalf("future command was misclassified: %+v", latest)
	}
}
