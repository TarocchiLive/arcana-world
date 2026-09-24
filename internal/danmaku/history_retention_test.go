package danmaku

import (
	"fmt"
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
	events, err := h.Range(expired.Add(-time.Second), time.Now(), 0, nil)
	if err != nil || len(events) != 1 || !reflect.DeepEqual(events[0], kept) {
		t.Fatalf("retention left an expired time index: events=%+v err=%v", events, err)
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

func TestRetentionAcrossPagesPreservesRecentEvents(t *testing.T) {
	dir := t.TempDir()
	h := openHistory(t, dir)
	now := time.Now().UTC()
	projections := make([]projection, 512)
	want := 0
	for i := range projections {
		at := now.Add(-retention - time.Hour)
		if i%7 == 0 {
			at = now
			want++
		}
		projections[i].event = Event{RoomID: 1, Kind: "chat", Text: fmt.Sprintf("event-%d", i), Time: at}
	}
	if _, err := h.append(1, []byte(`{"cmd":"DANMU_MSG"}`), projections...); err != nil {
		t.Fatal(err)
	}
	if err := h.prune(now); err != nil {
		t.Fatal(err)
	}
	events := page(t, h, 1, 0, len(projections))
	if len(events) != want {
		t.Fatalf("retention kept %d events, want %d", len(events), want)
	}
	for _, event := range events {
		if !event.Time.Equal(now) {
			t.Fatalf("expired event survived retention: %d", event.Sequence)
		}
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h = openHistory(t, dir)
	if err := h.prune(now.Add(retention + time.Hour)); err != nil {
		t.Fatal(err)
	}
	if events := page(t, h, 1, 0, len(projections)); len(events) != 0 {
		t.Fatalf("final retention left %d events", len(events))
	}
}
