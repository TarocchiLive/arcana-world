package danmaku

import (
	"reflect"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestRetentionRecoversExpiredOrphans(t *testing.T) {
	dir := t.TempDir()
	h := openHistory(t, dir)
	expired := time.Now().UTC().Add(-retention - time.Hour)
	for range 2 {
		if _, err := h.append(1, []byte(`{"kind":"gap","text":"session_end"}`), projection{event: Event{RoomID: 1, Kind: "gap", Text: "session_end", Time: expired}}); err != nil {
			t.Fatal(err)
		}
	}
	appendEvent(t, h, 1, chat("retained"), true)
	kept := page(t, h, 1, 0, 1)[0]
	// 复现原始消息已释放、过期事件仍保留的旧库状态。
	if err := h.db.Update(func(tx *bolt.Tx) error {
		room := tx.Bucket(roomsBucket).Bucket(key(1))
		for _, seq := range []uint64{1, 2} {
			if err := releaseMessage(room, key(seq)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h = openHistory(t, dir)
	if events := page(t, h, 1, 0, 10); len(events) != 1 || !reflect.DeepEqual(events[0], kept) {
		t.Fatalf("retention changed recent history or kept expired records: %+v", events)
	}
	if err := h.db.View(func(tx *bolt.Tx) error {
		room := tx.Bucket(roomsBucket).Bucket(key(1))
		if raw := rawForEvent(room, key(kept.Sequence)); string(raw) != chat("retained") {
			t.Fatalf("retained payload changed: %q", raw)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	appendEvent(t, h, 1, chat("after-recovery"), true)
	if events := page(t, h, 1, 0, 10); len(events) != 2 || events[0].Sequence <= kept.Sequence {
		t.Fatalf("cannot append after recovery: %+v", events)
	}
}
