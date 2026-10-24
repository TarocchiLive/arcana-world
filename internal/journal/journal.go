// journal 包将调试日志无缓冲地持久化到数据目录，记录前隐藏已登记的机密值。
package journal

import (
	"arcana-world/internal/i18n"

	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	bolt "go.etcd.io/bbolt"
)

// Log 串行处理记录写入和关闭；同一进程内最近打开的 Log 接收 Logger 输出。
type Log struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	closeErr error
	writeErr error
	lock     io.Closer
	stop     chan struct{}
}

// active 指向当前接收调试记录的日志；为空时 Logger 直接丢弃记录。
var active atomic.Pointer[Log]

// Open 在 dataDir 下打开仅追加日志，并确保目录和文件权限安全。
func Open(dataDir string) (*Log, error) {
	base, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalDirectoryResolveFailed), err)
	}
	lock, err := LockDirectory(base)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, "logs")
	keepLock := false
	defer func() {
		if !keepLock {
			_ = lock.Close()
		}
	}()
	// 重命名前崩溃可能遗留尚未提交的私有副本。
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".journal-") && !entry.IsDir() {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return nil, err
			}
		}
	}

	path := filepath.Join(dir, "arcana-world.log")
	before, err := os.Lstat(path)
	flags := os.O_RDWR | os.O_APPEND
	if errors.Is(err, os.ErrNotExist) {
		flags |= os.O_CREATE | os.O_EXCL
	} else if err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalFileInspectFailed), err)
	} else if !before.Mode().IsRegular() {
		return nil, errors.New(i18n.T(i18n.JournalRegularFileRequired))
	}
	file, err := openLogFile(path, flags)
	if err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalOpenFailed), err)
	}
	fail := func(cause error) (*Log, error) {
		return nil, errors.Join(cause, file.Close())
	}
	opened, err := file.Stat()
	if err != nil {
		return fail(fmt.Errorf(i18n.T(i18n.JournalOpenedFileInspectFailed), err))
	}
	current, err := os.Lstat(path)
	if err != nil {
		return fail(fmt.Errorf(i18n.T(i18n.JournalPathInspectFailed), err))
	}
	if !opened.Mode().IsRegular() || !current.Mode().IsRegular() ||
		!os.SameFile(opened, current) || (before != nil && !os.SameFile(before, opened)) {
		return fail(errors.New(i18n.T(i18n.JournalFileChanged)))
	}
	if err := file.Chmod(0600); err != nil {
		return fail(fmt.Errorf(i18n.T(i18n.JournalFileSecureFailed), err))
	}
	l := &Log{file: file, path: path, lock: lock, stop: make(chan struct{})}
	if err := l.prune(time.Now()); err != nil {
		return nil, errors.Join(err, l.file.Close())
	}
	keepLock = true
	go l.retain()
	active.Store(l)
	return l, nil
}

// LockDirectory 复用日志的独占目录锁，不打开日志或启动后台任务。
// 调用方必须在完成配置读写后关闭锁。
func LockDirectory(dataDir string) (io.Closer, error) {
	base, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalDirectoryResolveFailed), err)
	}
	if info, err := os.Lstat(base); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New(i18n.T(i18n.JournalDataDirectoryRequired))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf(i18n.T(i18n.JournalDataDirectoryInspectFailed), err)
	}
	if err := os.MkdirAll(base, 0700); err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalDirectoryCreateFailed), err)
	}
	info, err := os.Lstat(base)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New(i18n.T(i18n.JournalDataDirectoryRequired))
	}
	if err := os.Chmod(base, 0700); err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalDirectorySecureFailed), err)
	}
	dir := filepath.Join(base, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalDirectoryCreateFailed), err)
	}
	info, err = os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalDirectoryInspectFailed), err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New(i18n.T(i18n.JournalDirectoryRequired))
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalDirectorySecureFailed), err)
	}
	// 独立且稳定的锁 inode 同时保护跨进程的日志原子替换和写入。
	// Bolt 提供跨平台的咨询锁。
	lockPath := filepath.Join(dir, "journal.lock")
	if info, err := os.Lstat(lockPath); err == nil {
		if !info.Mode().IsRegular() {
			return nil, errors.New(i18n.T(i18n.JournalRegularFileRequired))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	lock, err := bolt.Open(lockPath, 0600, &bolt.Options{Timeout: time.Second})
	if err != nil {
		if errors.Is(err, bolt.ErrTimeout) {
			return nil, fmt.Errorf(i18n.T(i18n.DataDirectoryOccupied), base, err)
		}
		return nil, err
	}
	return lock, nil
}

// Path 返回日志文件的绝对路径。
func (l *Log) Path() string { return l.path }

// timestampLayout 保留毫秒以便对齐并发组件的先后顺序；prune 仍按 RFC 3339 解析。
const timestampLayout = "2006-01-02T15:04:05.000Z07:00"

// writeRecord 为一条已格式化且以换行结尾的记录加上时间戳并追加写入。
// 写入失败只保留首个错误并在 Close 时返回，避免日志故障打断业务流程。
func (l *Log) writeRecord(record []byte) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	line := make([]byte, 0, len(timestampLayout)+1+len(record))
	line = time.Now().AppendFormat(line, timestampLayout)
	line = append(line, ' ')
	line = append(line, record...)
	n, err := l.file.Write(line)
	if err == nil && n != len(line) {
		err = io.ErrShortWrite
	}
	if err != nil && l.writeErr == nil {
		l.writeErr = fmt.Errorf("journal: write %s: %w", l.path, err)
	}
}

// Close 只同步并关闭日志一次，重复调用时保留此前的错误。
func (l *Log) Close() error {
	active.CompareAndSwap(l, nil)
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		close(l.stop)
		l.closeErr = errors.Join(l.closeErr, l.writeErr, l.file.Sync(), l.file.Close(), l.lock.Close())
		l.file = nil
	}
	return l.closeErr
}

const retention = 7 * 24 * time.Hour
const retentionInterval = 15 * time.Minute

func (l *Log) retain() {
	ticker := time.NewTicker(retentionInterval)
	defer ticker.Stop()
	for {
		select {
		case now := <-ticker.C:
			l.mu.Lock()
			if l.file != nil {
				if err := l.prune(now); err != nil {
					l.closeErr = errors.Join(l.closeErr, err)
				}
			}
			l.mu.Unlock()
		case <-l.stop:
			return
		}
	}
}

// prune 要求持有写入互斥锁（或在启动时独占尚未对外提供的 Log）。
// 它将完整的物理记录流式写入私有文件，同步后再原子替换路径。
// 复制失败绝不会截断原日志。
func (l *Log) prune(now time.Time) error {
	if _, err := l.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	reader := bufio.NewReader(l.file)
	cutoff := now.Add(-retention)
	var replacement *os.File
	var replacementPath string
	defer func() {
		if replacement != nil {
			_ = replacement.Close()
			_ = os.Remove(replacementPath)
		}
	}()
	var offset int64
	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return err
		}
		if len(line) == 0 {
			break
		}
		stamp, _, found := strings.Cut(line, " ")
		at, parseErr := time.Parse(time.RFC3339, stamp)
		expired := found && parseErr == nil && at.Before(cutoff)
		if expired && replacement == nil {
			replacement, err = os.CreateTemp(filepath.Dir(l.path), ".journal-*")
			if err != nil {
				return err
			}
			replacementPath = replacement.Name()
			if _, err := io.CopyN(replacement, io.NewSectionReader(l.file, 0, offset), offset); err != nil {
				return err
			}
		}
		if replacement != nil && !expired {
			if _, err := replacement.WriteString(line); err != nil {
				return err
			}
		}
		offset += int64(len(line))
		if errors.Is(err, io.EOF) {
			break
		}
	}
	if replacement == nil {
		return nil
	}
	err := l.commitReplacement(replacement)
	replacement = nil
	return err
}
