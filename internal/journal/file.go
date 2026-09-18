package journal

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
)

// commitReplacement 接管临时副本的所有权。替换失败时，原日志仍可写入；
// 替换成功后，即使关闭旧句柄或同步目录失败，后续写入也使用新文件。
// 调用方必须持有写锁，或独占尚未对外提供的 Log。
func (l *Log) commitReplacement(replacement *os.File) error {
	path := replacement.Name()
	defer func() {
		_ = replacement.Close()
		_ = os.Remove(path)
	}()
	if err := replacement.Sync(); err != nil {
		return err
	}
	if err := replacement.Close(); err != nil {
		return err
	}
	next, err := openLogFile(path, os.O_RDWR|os.O_APPEND)
	if err != nil {
		return err
	}
	if err := replaceLogFile(next, l.path); err != nil {
		return errors.Join(err, next.Close())
	}
	previous := l.file
	l.file = next
	err = previous.Close()
	if runtime.GOOS != "windows" {
		directory, openErr := os.Open(filepath.Dir(l.path))
		if openErr != nil {
			return errors.Join(err, openErr)
		}
		err = errors.Join(err, directory.Sync(), directory.Close())
	}
	return err
}
