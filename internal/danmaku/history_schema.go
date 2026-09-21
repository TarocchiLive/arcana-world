package danmaku

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	bolt "go.etcd.io/bbolt"
)

const historySchemaVersion uint64 = 1

var historyMetaBucket = []byte("schema")
var historyVersionKey = []byte("version")
var legacyRawBucket = []byte("raw")

// migrateHistory 在保留期清理前执行，在单个事务中处理所有直播间。
// 仅在所有旧引用转换完成后提交版本标记。
func migrateHistory(tx *bolt.Tx) error {
	meta, err := tx.CreateBucketIfNotExists(historyMetaBucket)
	if err != nil {
		return err
	}
	var version uint64
	if encoded := meta.Get(historyVersionKey); encoded != nil {
		if len(encoded) != 8 {
			return errors.New("invalid history schema version")
		}
		version = binary.BigEndian.Uint64(encoded)
	}
	if version > historySchemaVersion {
		return fmt.Errorf("history schema version %d is newer than supported version %d", version, historySchemaVersion)
	}
	rooms, err := tx.CreateBucketIfNotExists(roomsBucket)
	if err != nil {
		return err
	}
	if version == historySchemaVersion {
		return nil
	}
	if err := rooms.ForEach(func(roomKey, value []byte) error {
		if value != nil {
			return errors.New("invalid history room bucket")
		}
		return migrateRoom(rooms.Bucket(roomKey))
	}); err != nil {
		return err
	}
	return meta.Put(historyVersionKey, key(historySchemaVersion))
}

func migrateRoom(room *bolt.Bucket) error {
	events, legacy := room.Bucket(eventsBucket), room.Bucket(legacyRawBucket)
	if events == nil || legacy == nil {
		return errors.New("legacy history is missing events or raw payloads")
	}
	for _, name := range [][]byte{rawBucket, messageIDsBucket, messageRefsBucket} {
		if _, err := room.CreateBucket(name); err != nil {
			return err
		}
	}
	messages, ids, refs := room.Bucket(rawBucket), room.Bucket(messageIDsBucket), room.Bucket(messageRefsBucket)
	var previousRaw []byte
	var received time.Time
	var batch []projection
	var messageID []byte
	var references uint64
	lastIndex := -1
	if err := events.ForEach(func(seq, encoded []byte) error {
		var event Event
		if len(seq) != 8 || encoded == nil {
			return errors.New("invalid legacy history event")
		}
		if err := json.Unmarshal(encoded, &event); err != nil {
			return err
		}
		raw := legacy.Get(seq)
		if raw == nil {
			return fmt.Errorf("legacy history event %d is missing its raw payload", event.Sequence)
		}
		// 仅凭字节无法标识一次接收。只合并接收时间戳相同、
		// 条目顺序递增且相邻的批次映射记录。
		// 重放、单条事件及无法确定归属的旧映射记录仍单独保留。
		sameReceive := messageID != nil && event.Time.Equal(received) && bytes.Equal(raw, previousRaw)
		if !sameReceive {
			batch = projectMany(event.RoomID, raw)
			lastIndex = -1
		}
		index := -1
		if len(batch) > 1 {
			for i, p := range batch {
				candidate := p.event
				candidate.Sequence, candidate.Time = event.Sequence, event.Time
				if reflect.DeepEqual(candidate, event) {
					index = i
					break
				}
			}
		}
		if !sameReceive || lastIndex < 0 || index < 0 || index <= lastIndex {
			id, err := messages.NextSequence()
			if err != nil {
				return err
			}
			messageID = key(id)
			references = 0
			if err := messages.Put(messageID, raw); err != nil {
				return err
			}
		}
		previousRaw, received, lastIndex = raw, event.Time, index
		references++
		if err := ids.Put(seq, messageID); err != nil {
			return err
		}
		return refs.Put(messageID, key(references))
	}); err != nil {
		return err
	}
	return room.DeleteBucket(legacyRawBucket)
}

func rawForEvent(room *bolt.Bucket, sequence []byte) []byte {
	return room.Bucket(rawBucket).Get(room.Bucket(messageIDsBucket).Get(sequence))
}

func releaseMessage(room *bolt.Bucket, sequence []byte) error {
	ids, refs, messages := room.Bucket(messageIDsBucket), room.Bucket(messageRefsBucket), room.Bucket(rawBucket)
	id := ids.Get(sequence)
	if len(id) != 8 {
		return errors.New("history event is missing its message ID")
	}
	encoded := refs.Get(id)
	if len(encoded) != 8 || binary.BigEndian.Uint64(encoded) == 0 {
		return errors.New("invalid history message reference count")
	}
	count := binary.BigEndian.Uint64(encoded)
	if count == 1 {
		if err := messages.Delete(id); err != nil {
			return err
		}
		if err := refs.Delete(id); err != nil {
			return err
		}
	} else if err := refs.Put(id, key(count-1)); err != nil {
		return err
	}
	return ids.Delete(sequence)
}
