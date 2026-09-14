// journal 包无缓冲地持久化经过脱敏的应用事件。
package journal

import (
	"arcana-world/internal/i18n"

	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// Log 串行处理事件写入和关闭；调用方必须在 Write 前移除机密信息。
type Log struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	closeErr error
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

	path := filepath.Join(dir, "arcana-world.log")
	before, err := os.Lstat(path)
	flags := os.O_WRONLY | os.O_APPEND
	if errors.Is(err, os.ErrNotExist) {
		flags |= os.O_CREATE | os.O_EXCL
	} else if err != nil {
		return nil, fmt.Errorf(i18n.T(i18n.JournalFileInspectFailed), err)
	} else if !before.Mode().IsRegular() {
		return nil, errors.New(i18n.T(i18n.JournalRegularFileRequired))
	}
	file, err := os.OpenFile(path, flags, 0600)
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
	return &Log{file: file, path: path}, nil
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
		l.closeErr = errors.Join(l.file.Sync(), l.file.Close())
		l.file = nil
	}
	return l.closeErr
}
