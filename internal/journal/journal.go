// Package journal persists sanitized application events without buffering.
package journal

import (
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

// Log serializes event writes and shutdown. Callers must redact secrets before Write.
type Log struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	closeErr error
}

// Open opens an append-only journal beneath dataDir, securing its directory and file.
func Open(dataDir string) (*Log, error) {
	base, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, fmt.Errorf("resolve journal directory: %w", err)
	}
	if info, err := os.Lstat(base); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("journal data path must be a directory, not a symlink")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect journal data directory: %w", err)
	}
	dir := filepath.Join(base, "logs")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create journal directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect journal directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("journal path must be a directory, not a symlink")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, fmt.Errorf("secure journal directory: %w", err)
	}

	path := filepath.Join(dir, "arcana-world.log")
	before, err := os.Lstat(path)
	flags := os.O_WRONLY | os.O_APPEND
	if errors.Is(err, os.ErrNotExist) {
		flags |= os.O_CREATE | os.O_EXCL
	} else if err != nil {
		return nil, fmt.Errorf("inspect journal file: %w", err)
	} else if !before.Mode().IsRegular() {
		return nil, errors.New("journal must be a regular file, not a symlink")
	}
	file, err := os.OpenFile(path, flags, 0600)
	if err != nil {
		return nil, fmt.Errorf("open journal: %w", err)
	}
	fail := func(cause error) (*Log, error) {
		return nil, errors.Join(cause, file.Close())
	}
	opened, err := file.Stat()
	if err != nil {
		return fail(fmt.Errorf("inspect opened journal: %w", err))
	}
	current, err := os.Lstat(path)
	if err != nil {
		return fail(fmt.Errorf("inspect journal path: %w", err))
	}
	if !opened.Mode().IsRegular() || !current.Mode().IsRegular() ||
		!os.SameFile(opened, current) || (before != nil && !os.SameFile(before, opened)) {
		return fail(errors.New("journal file changed while opening"))
	}
	if err := file.Chmod(0600); err != nil {
		return fail(fmt.Errorf("secure journal file: %w", err))
	}
	return &Log{file: file, path: path}, nil
}

// Path returns the absolute journal filename.
func (l *Log) Path() string { return l.path }

// Write appends one timestamped physical line. It strips terminal sequences and
// control/format characters, and escapes line breaks rather than creating records.
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

// Close syncs and closes the journal once, preserving any error on repeated calls.
func (l *Log) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		l.closeErr = errors.Join(l.file.Sync(), l.file.Close())
		l.file = nil
	}
	return l.closeErr
}
