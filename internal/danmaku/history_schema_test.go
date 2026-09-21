package danmaku

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

// downgradeHistory 生成实际使用过的无版本、以 seq 为键的原始数据格式。
func downgradeHistory(t *testing.T, h *History) {
	t.Helper()
	if err := h.db.Update(func(tx *bolt.Tx) error {
		rooms := tx.Bucket(roomsBucket)
		if err := rooms.ForEach(func(k, _ []byte) error {
			room := rooms.Bucket(k)
			legacy, err := room.CreateBucket(legacyRawBucket)
			if err != nil {
				return err
			}
			if err := room.Bucket(eventsBucket).ForEach(func(seq, _ []byte) error {
				return legacy.Put(seq, rawForEvent(room, seq))
			}); err != nil {
				return err
			}
			for _, name := range [][]byte{rawBucket, messageIDsBucket, messageRefsBucket} {
				if err := room.DeleteBucket(name); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			return err
		}
		return tx.DeleteBucket(historyMetaBucket)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryMigrationPreservesReceivesAndPagination(t *testing.T) {
	dir := t.TempDir()
	h := openHistory(t, dir)
	raw := giftV2Fixture(giftV2Item(1), giftV2Item(2))
	appendEvent(t, h, 1, raw, true)
	for range 2 {
		appendEvent(t, h, 1, chat(""), true)
	}
	received := time.Now().UTC().Add(-time.Hour)
	unknown := []byte(`{"cmd":"WATCHED_CHANGE","data":{"num":42}}`)
	if _, err := h.append(1, unknown, projection{event: Event{RoomID: 1, Kind: "unknown", Time: received}}); err != nil {
		t.Fatal(err)
	}
	downgradeHistory(t, h)
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h = openHistory(t, dir)
	appendEvent(t, h, 1, raw, false)
	if err := h.db.View(func(tx *bolt.Tx) error {
		room := tx.Bucket(roomsBucket).Bucket(key(1))
		if room.Bucket(legacyRawBucket) != nil || room.Bucket(rawBucket).Stats().KeyN != 4 {
			return errors.New("migration did not coalesce only the batch payload")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	events := page(t, h, 1, 0, 1)
	if len(events) != 1 || events[0].Sequence != 5 || events[0].Kind != "watched" || events[0].Count != 42 || !events[0].Time.Equal(received) {
		t.Fatalf("migrated unknown: %+v", events)
	}
	events = page(t, h, 1, 5, 10)
	if len(events) != 4 || events[0].Sequence != 4 || events[1].Sequence != 3 || events[2].Count != 2 || events[3].Count != 1 {
		t.Fatalf("migrated page: %+v", events)
	}
}

func TestSharedMessageSurvivesPartialDedupAndRetention(t *testing.T) {
	h := openHistory(t, t.TempDir())
	appendEvent(t, h, 1, giftV2Fixture(giftV2Item(1)), true)
	raw := giftV2Fixture(giftV2Item(1), giftV2Item(2), giftV2Item(3))
	appendEvent(t, h, 1, raw, true)
	appendEvent(t, h, 1, raw, false)
	now := time.Now().UTC()
	if err := h.db.Update(func(tx *bolt.Tx) error {
		room := tx.Bucket(roomsBucket).Bucket(key(1))
		if room.Bucket(rawBucket).Stats().KeyN != 2 {
			return errors.New("batch raw stored more than once")
		}
		events := room.Bucket(eventsBucket)
		for _, seq := range []uint64{1, 2} {
			var event Event
			if err := json.Unmarshal(events.Get(key(seq)), &event); err != nil {
				return err
			}
			event.Time = now.Add(-retention - time.Hour)
			encoded, err := json.Marshal(event)
			if err != nil {
				return err
			}
			if err := events.Put(key(seq), encoded); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.prune(now); err != nil {
		t.Fatal(err)
	}
	if events := page(t, h, 1, 0, 10); len(events) != 1 || events[0].Count != 3 {
		t.Fatalf("partial retention: %+v", events)
	}
	if err := h.db.View(func(tx *bolt.Tx) error {
		room := tx.Bucket(roomsBucket).Bucket(key(1))
		if room.Bucket(rawBucket).Stats().KeyN != 1 || string(rawForEvent(room, key(3))) != raw {
			return errors.New("partial retention discarded a shared payload")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.prune(now.Add(retention + time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := h.db.View(func(tx *bolt.Tx) error {
		room := tx.Bucket(roomsBucket).Bucket(key(1))
		if room.Bucket(rawBucket).Stats().KeyN != 0 || room.Bucket(messageIDsBucket).Stats().KeyN != 0 || room.Bucket(messageRefsBucket).Stats().KeyN != 0 {
			return errors.New("last projection left orphaned message data")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestHistoryMigrationRollbackAndFutureVersion(t *testing.T) {
	dir := t.TempDir()
	h := openHistory(t, dir)
	appendEvent(t, h, 1, chat("a"), true)
	appendEvent(t, h, 1, chat("b"), true)
	downgradeHistory(t, h)
	if err := h.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(roomsBucket).Bucket(key(1)).Bucket(legacyRawBucket).Delete(key(2))
	}); err != nil {
		t.Fatal(err)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(dir); err == nil {
		opened.Close()
		t.Fatal("migration accepted a missing payload")
	}
	db, err := bolt.Open(filepath.Join(dir, "danmaku", "history.db"), 0600, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Update(func(tx *bolt.Tx) error {
		room := tx.Bucket(roomsBucket).Bucket(key(1))
		if tx.Bucket(historyMetaBucket) != nil || room.Bucket(rawBucket) != nil || string(room.Bucket(legacyRawBucket).Get(key(1))) != chat("a") {
			return errors.New("failed migration was not rolled back")
		}
		meta, err := tx.CreateBucket(historyMetaBucket)
		if err != nil {
			return err
		}
		return meta.Put(historyVersionKey, key(historySchemaVersion+1))
	}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if opened, err := Open(dir); err == nil {
		opened.Close()
		t.Fatal("opened an unsupported future schema")
	}
}
