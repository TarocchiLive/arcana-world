// journal 包无缓冲地持久化经过脱敏的应用事件。
package journal

import (
	"arcana-world/internal/i18n"

	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	bolt "go.etcd.io/bbolt"
)

// Log 串行处理事件写入和关闭；调用方必须在 Write 前移除机密信息。
type Log struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	closeErr error
	lock     *bolt.DB
	stop     chan struct{}
}

// Open 在 dataDir 下打开仅追加日志，并确保目录和文件权限安全。
func Open(dataDir string) (*Log, error) {
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
	dir := filepath.Join(base, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalDirectoryCreateFailed), err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalDirectoryInspectFailed), err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New(i18n.T(i18n.JournalDirectoryRequired))
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalDirectorySecureFailed), err)
	}

	// A separate, stable lock inode protects atomic log replacement across
	// processes as well as writes. Bolt supplies portable advisory locking.
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
		return nil, err
	}
	keepLock := false
	defer func() {
		if !keepLock {
			_ = lock.Close()
		}
	}()
	// A crash before rename may leave an uncommitted private copy.
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
	return l, nil
}

// Path 返回日志文件的绝对路径。
func (l *Log) Path() string { return l.path }

// Write 追加一条带时间戳的物理行，移除终端控制序列及控制和格式字符，
// 并转义换行符，避免将其误写成新的日志记录。
func (l *Log) Write(message string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return os.ErrClosed
	}
	var line strings.Builder
	line.Grow(len(message) + 40)
	line.WriteString(time.Now().Format(time.RFC3339))
	line.WriteByte(' ')
	for _, r := range ansi.Strip(message) {
		switch r {
		case '\n':
			line.WriteString(`\n`)
		case '\r':
			line.WriteString(`\r`)
		case '\u2028':
			line.WriteString(`\u2028`)
		case '\u2029':
			line.WriteString(`\u2029`)
		default:
			if !unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) {
				line.WriteRune(r)
			}
		}
	}
	line.WriteByte('\n')
	n, err := l.file.WriteString(line.String())
	if err == nil && n != line.Len() {
		err = io.ErrShortWrite
	}
	return err
}

// Close 只同步并关闭日志一次，重复调用时保留此前的错误。
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		close(l.stop)
		l.closeErr = errors.Join(l.closeErr, l.file.Sync(), l.file.Close(), l.lock.Close())
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

// prune requires the write mutex (or an unpublished Log at startup). It
// streams complete physical records into a private file, syncs it, then swaps
// the pathname atomically. A failed copy never truncates the original log.
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
	if err := replacement.Sync(); err != nil {
		return err
	}
	if err := replacement.Close(); err != nil {
		return err
	}
	// Acquire the append handle before committing the replacement.
	next, err := openLogFile(replacementPath, os.O_RDWR|os.O_APPEND)
	if err != nil {
		return err
	}
	if err := os.Rename(replacementPath, l.path); err != nil {
		return errors.Join(err, next.Close())
	}
	previous := l.file
	l.file = next
	err = previous.Close()
	replacement = nil
	if runtime.GOOS != "windows" {
		directory, openErr := os.Open(filepath.Dir(l.path))
		if openErr != nil {
			return errors.Join(err, openErr)
		}
		err = errors.Join(err, directory.Sync(), directory.Close())
	}
	return err
}
