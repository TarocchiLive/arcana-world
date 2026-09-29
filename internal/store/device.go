package store

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	bolt "go.etcd.io/bbolt"
)

const deviceFile = "device.db"

var deviceIDPattern = regexp.MustCompile(`^[0-9A-F]{8}-[0-9A-F]{4}-4[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}user$`)

// DeviceID 返回当前数据目录的固定请求标识，不随账号或设置重置改变。
func (s *Store) DeviceID() string { return s.deviceID }

func loadDeviceID(dir string) (string, error) {
	path := filepath.Join(dir, deviceFile)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() {
			return "", errors.New("invalid device identifier file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	// 复用日志使用的跨平台文件锁和事务，不依赖文件系统支持硬链接。
	db, err := bolt.Open(path, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		return "", err
	}
	var value string
	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists([]byte("device"))
		if err != nil {
			return err
		}
		if saved := bucket.Get([]byte("id")); saved != nil {
			if !deviceIDPattern.Match(saved) {
				return errors.New("invalid device identifier")
			}
			value = string(saved)
			return nil
		}
		var id [16]byte
		if _, err := rand.Read(id[:]); err != nil {
			return err
		}
		id[6] = (id[6] & 0x0f) | 0x40
		id[8] = (id[8] & 0x3f) | 0x80
		value = fmt.Sprintf("%X-%X-%X-%X-%Xuser", id[:4], id[4:6], id[6:8], id[8:10], id[10:])
		return bucket.Put([]byte("id"), []byte(value))
	})
	return value, errors.Join(err, db.Close())
}
