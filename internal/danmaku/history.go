package danmaku

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

// History 将关闭操作与事务串行化。Bolt 默认同步提交，
// 原子地保留原始字节、接收元数据、索引和删除标记。
type History struct {
	mu       sync.RWMutex
	db       *bolt.DB
	closeErr error
	stop     chan struct{}
}

var roomsBucket = []byte("rooms")
var eventsBucket = []byte("events")
var rawBucket = []byte("messages")
var messageIDsBucket = []byte("message-ids")
var messageRefsBucket = []byte("message-refs")
var idsBucket = []byte("identities")
var tombstonesBucket = []byte("sc-deleted")
var scBucket = []byte("sc-sequences")
var receivedBucket = []byte("received")

// 使用定长有符号秒数和纳秒编码，保证时钟调整后的时间排序。
func receivedKey(at time.Time, room, sequence uint64) []byte {
	var k [28]byte
	binary.BigEndian.PutUint64(k[:8], uint64(at.Unix())^(1<<63))
	binary.BigEndian.PutUint32(k[8:12], uint32(at.Nanosecond()))
	binary.BigEndian.PutUint64(k[12:20], room)
	binary.BigEndian.PutUint64(k[20:], sequence)
	return k[:]
}

func Open(dir string) (*History, error) {
	base, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if info, err := os.Lstat(base); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("history requires a real directory")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	dir = filepath.Join(base, "danmaku")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("history requires a real directory")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "history.db")
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New("history requires a regular file")
		}
		if err := os.Chmod(path, 0600); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return nil, err
	}
	if err := db.Update(migrateHistory); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	// 在 Unix 上，仅对文件执行 fsync 不会持久化新建的目录项。
	// Windows 无法通过 os.File 可移植地对目录执行 fsync；Bolt
	// 会使用其平台实现刷新数据库。
	if runtime.GOOS != "windows" {
		for _, directory := range []string{dir, base, filepath.Dir(base)} {
			f, err := os.Open(directory)
			if err != nil {
				return nil, errors.Join(err, db.Close())
			}
			err = errors.Join(f.Sync(), f.Close())
			if err != nil {
				return nil, errors.Join(err, db.Close())
			}
		}
	}
	h := &History{db: db, stop: make(chan struct{})}
	if err := h.prune(time.Now()); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	go h.retain()
	return h, nil
}
func (h *History) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.db != nil {
		close(h.stop)
		h.closeErr = errors.Join(h.closeErr, h.db.Close())
		h.db = nil
	}
	return h.closeErr
}
func key(n uint64) []byte { var b [8]byte; binary.BigEndian.PutUint64(b[:], n); return b[:] }

// Append 仅接受业务载荷；调用方绝不能传入认证帧、
// HTTP 请求头或连接凭据。格式错误的业务字节也会保留。
func (h *History) Append(roomID int64, raw json.RawMessage) (bool, error) {
	return h.append(roomID, raw, projectMany(roomID, raw)...)
}
func (h *History) append(roomID int64, raw []byte, projections ...projection) (bool, error) {
	if roomID <= 0 {
		return false, errors.New("history room ID must be positive")
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.db == nil {
		return false, os.ErrClosed
	}
	inserted := false
	err := h.db.Update(func(tx *bolt.Tx) error {
		room, err := tx.Bucket(roomsBucket).CreateBucketIfNotExists(key(uint64(roomID)))
		if err != nil {
			return err
		}
		for _, name := range [][]byte{eventsBucket, rawBucket, messageIDsBucket, messageRefsBucket, idsBucket, tombstonesBucket, scBucket} {
			if _, err := room.CreateBucketIfNotExists(name); err != nil {
				return err
			}
		}
		events, ids, tombstones, sc := room.Bucket(eventsBucket), room.Bucket(idsBucket), room.Bucket(tombstonesBucket), room.Bucket(scBucket)
		var messageID []byte
		var references uint64
		for _, p := range projections {
			if p.identity != "" && ids.Get([]byte(p.identity)) != nil {
				continue
			}
			seq, err := events.NextSequence()
			if err != nil {
				return err
			}
			for _, id := range p.deletes {
				if err := tombstones.Put([]byte(id), key(seq)); err != nil {
					return err
				}
				if seq := sc.Get([]byte(id)); seq != nil {
					var event Event
					if err := json.Unmarshal(events.Get(seq), &event); err != nil {
						return err
					}
					event.Deleted, event.Text = true, ""
					encoded, err := json.Marshal(event)
					if err != nil {
						return err
					}
					if err := events.Put(seq, encoded); err != nil {
						return err
					}
				}
			}
			p.event.Sequence = seq
			if p.scID != "" && tombstones.Get([]byte(p.scID)) != nil {
				p.event.Deleted, p.event.Text = true, ""
			}
			encoded, err := json.Marshal(p.event)
			if err != nil {
				return err
			}
			seqKey := key(seq)
			if err := tx.Bucket(receivedBucket).Put(receivedKey(p.event.Time, uint64(roomID), seq), nil); err != nil {
				return err
			}
			if err := events.Put(seqKey, encoded); err != nil {
				return err
			}
			if messageID == nil {
				id, err := room.Bucket(rawBucket).NextSequence()
				if err != nil {
					return err
				}
				messageID = key(id)
				if err := room.Bucket(rawBucket).Put(messageID, raw); err != nil {
					return err
				}
			}
			if err := room.Bucket(messageIDsBucket).Put(seqKey, messageID); err != nil {
				return err
			}
			references++
			if p.identity != "" {
				if err := ids.Put([]byte(p.identity), seqKey); err != nil {
					return err
				}
			}
			if p.scID != "" {
				if err := sc.Put([]byte(p.scID), seqKey); err != nil {
					return err
				}
			}
			inserted = true
		}
		if messageID != nil {
			return room.Bucket(messageRefsBucket).Put(messageID, key(references))
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	return inserted, nil
}

// Page 按从新到旧的顺序返回序号严格小于 before 的记录；零表示从最新记录开始。
// limit 非正时返回空页，不分配大缓冲区。
func (h *History) Page(roomID int64, before uint64, limit int) ([]Event, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.db == nil {
		return nil, os.ErrClosed
	}
	var result []Event
	if limit <= 0 {
		return result, nil
	}
	err := h.db.View(func(tx *bolt.Tx) error {
		room := tx.Bucket(roomsBucket).Bucket(key(uint64(roomID)))
		if room == nil {
			return nil
		}
		c := room.Bucket(eventsBucket).Cursor()
		k, v := c.Last()
		if before != 0 {
			k, v = c.Seek(key(before))
			if k == nil {
				k, v = c.Last()
			} else {
				k, v = c.Prev()
			}
		}
		for ; k != nil && len(result) < limit; k, v = c.Prev() {
			event, err := decodeHistoryEvent(room, roomID, k, v)
			if err != nil {
				return err
			}
			result = append(result, event)
		}
		return nil
	})
	return result, err
}

func decodeHistoryEvent(room *bolt.Bucket, roomID int64, k, v []byte) (Event, error) {
	var event Event
	if err := json.Unmarshal(v, &event); err != nil {
		return Event{}, err
	}
	// 重新解释旧的未知记录，不改变其接收时间、
	// 稳定的分页游标或已保留的原始载荷。
	if event.Kind == "unknown" && !event.Deleted {
		p := project(roomID, rawForEvent(room, k))
		if p.event.Kind != "unknown" && len(p.batch) == 0 && p.scID == "" && len(p.deletes) == 0 {
			p.event.Sequence, p.event.Time = event.Sequence, event.Time
			event = p.event
		}
	}
	if event.Kind == "guard" && event.GuardUnit == "" {
		event.GuardUnit = project(roomID, rawForEvent(room, k)).event.GuardUnit
	}
	return event, nil
}

// Range 返回所有直播间在接收时间闭区间内的记录，按时间、直播间和序号降序排列。
// visible 为 nil 时不过滤；limit 为零时不限数量，为正时仅保留过滤后的最新记录。
func (h *History) Range(start, end time.Time, limit int, visible func(Event) bool) ([]Event, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.db == nil {
		return nil, os.ErrClosed
	}
	if end.Before(start) {
		return nil, errors.New("history range end precedes start")
	}
	if limit < 0 {
		return nil, errors.New("history range limit must not be negative")
	}
	var result []Event
	err := h.db.View(func(tx *bolt.Tx) error {
		cursor := tx.Bucket(receivedBucket).Cursor()
		upper := receivedKey(end, ^uint64(0), ^uint64(0))
		k, _ := cursor.Seek(upper)
		if k == nil {
			k, _ = cursor.Last()
		} else if string(k) > string(upper) {
			k, _ = cursor.Prev()
		}
		lower := receivedKey(start, 0, 0)
		for ; k != nil && string(k) >= string(lower) && (limit == 0 || len(result) < limit); k, _ = cursor.Prev() {
			roomID := binary.BigEndian.Uint64(k[12:20])
			room := tx.Bucket(roomsBucket).Bucket(key(roomID))
			seq := k[20:]
			event, err := decodeHistoryEvent(room, int64(roomID), seq, room.Bucket(eventsBucket).Get(seq))
			if err != nil {
				return err
			}
			if visible == nil || visible(event) {
				result = append(result, event)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (h *History) RecordGap(roomID int64, text string) error {
	// 仅接受调用方编写的状态文本，绝不接受含凭据的原始连接错误。
	raw, err := json.Marshal(struct {
		Kind string `json:"kind"`
		Text string `json:"text"`
	}{"gap", text})
	if err != nil {
		return err
	}
	_, err = h.append(roomID, raw, projection{event: Event{RoomID: roomID, Kind: "gap", Text: text, Time: time.Now().UTC()}})
	return err
}
func (h *History) Rooms() ([]int64, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.db == nil {
		return nil, os.ErrClosed
	}
	var rooms []int64
	err := h.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(roomsBucket).ForEach(func(k, v []byte) error {
			if v == nil && len(k) == 8 {
				rooms = append(rooms, int64(binary.BigEndian.Uint64(k)))
			}
			return nil
		})
	})
	return rooms, err
}

const retention = 7 * 24 * time.Hour
const retentionInterval = 15 * time.Minute

func (h *History) retain() {
	ticker := time.NewTicker(retentionInterval)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			if err := h.prune(now); err != nil && !errors.Is(err, os.ErrClosed) {
				h.mu.Lock()
				h.closeErr = errors.Join(h.closeErr, err)
				h.mu.Unlock()
			}
		case <-h.stop:
			return
		}
	}
}

// prune 在同一事务中删除原始载荷及其映射结果。Bolt
// 复用释放的页；不会替换正在使用的数据库或将其原地重写。
func (h *History) prune(now time.Time) error {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.db == nil {
		return os.ErrClosed
	}
	cutoff := now.Add(-retention)
	return h.db.Update(func(tx *bolt.Tx) error {
		rooms := tx.Bucket(roomsBucket)
		return rooms.ForEach(func(roomKey, value []byte) error {
			if value != nil {
				return nil
			}
			room := rooms.Bucket(roomKey)
			events := room.Bucket(eventsBucket)
			tombstones := room.Bucket(tombstonesBucket)
			// 旧数据库存储的是单字节标记，而非删除序号。
			// 从保留的业务字节中恢复这些引用。
			legacy := false
			if err := tombstones.ForEach(func(_, v []byte) error {
				legacy = legacy || len(v) != 8
				return nil
			}); err != nil {
				return err
			}
			c := events.Cursor()
			for k, v := c.First(); k != nil; k, v = c.Next() {
				var event Event
				if err := json.Unmarshal(v, &event); err != nil {
					return err
				}
				if event.Time.Before(cutoff) {
					if err := tx.Bucket(receivedBucket).Delete(receivedKey(event.Time, uint64(event.RoomID), event.Sequence)); err != nil {
						return err
					}
					if err := releaseMessage(room, k); err != nil {
						return err
					}
					if err := c.Delete(); err != nil {
						return err
					}
				} else if legacy {
					for _, p := range projectMany(event.RoomID, rawForEvent(room, k)) {
						for _, id := range p.deletes {
							if err := tombstones.Put([]byte(id), k); err != nil {
								return err
							}
						}
					}
				}
			}
			for _, name := range [][]byte{idsBucket, scBucket, tombstonesBucket} {
				c := room.Bucket(name).Cursor()
				for k, seq := c.First(); k != nil; k, seq = c.Next() {
					if len(seq) != 8 || events.Get(seq) == nil {
						if err := c.Delete(); err != nil {
							return err
						}
					}
				}
			}
			return nil
		})
	})
}
