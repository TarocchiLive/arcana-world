package journal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func readEvents(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("journal is missing its final record delimiter: %q", data)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	events := make([]string, len(lines))
	for i, line := range lines {
		stamp, event, ok := strings.Cut(line, " ")
		if !ok {
			t.Fatalf("record has no timestamp separator: %q", line)
		}
		if _, err := time.Parse(time.RFC3339, stamp); err != nil {
			t.Fatalf("invalid timestamp %q: %v", stamp, err)
		}
		events[i] = event
	}
	return events
}

func TestAppendAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	log, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	if err := log.Write("first event"); err != nil {
		t.Fatal(err)
	}
	// 关闭前读取也必须能看到事件：日志并非仅在退出时才刷新缓冲。
	if events := readEvents(t, log.Path()); len(events) != 1 || events[0] != "first event" {
		t.Fatalf("events before close: %q", events)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if err := log.Write("after close"); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("write after close: %v", err)
	}
	log, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Write("second event"); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, log.Path())
	if len(events) != 2 || events[0] != "first event" || events[1] != "second event" {
		t.Fatalf("reopened journal lost or replaced events: %q", events)
	}
}

func TestWritePreventsRecordAndTerminalInjection(t *testing.T) {
	log, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	message := "开始\nforged\r\n\x1b[31mred\x1b[0m\x1b]0;hidden title\a\x00\b\t\u2028\u2029\u202eend"
	if err := log.Write(message); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, log.Path())
	want := `开始\nforged\r\nred\u2028\u2029end`
	if len(events) != 1 || events[0] != want {
		t.Fatalf("unsafe or altered event: %q; want %q", events, want)
	}
}

func TestConcurrentWritesAndClose(t *testing.T) {
	log, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	const count = 64
	results := make(chan string, count*2)
	errs := make(chan error, count*2+1)
	start := make(chan struct{})
	var workers sync.WaitGroup
	var ready sync.WaitGroup
	ready.Add(count)
	for i := range count {
		workers.Add(1)
		go func(i int) {
			defer workers.Done()
			first := fmt.Sprintf("before-close-%d", i)
			if err := log.Write(first); err != nil {
				t.Errorf("write before close: %v", err)
			} else {
				results <- first
			}
			ready.Done()
			<-start
			message := fmt.Sprintf("event-%d", i)
			if err := log.Write(message); err != nil {
				errs <- err
			} else {
				results <- message
			}
		}(i)
	}
	// 即使 Close 抢在每次并发写入之前执行，也要保证日志非空。
	if err := log.Write("initial"); err != nil {
		t.Fatal(err)
	}
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-start
		if err := log.Close(); err != nil {
			errs <- err
		}
	}()
	ready.Wait()
	close(start)
	workers.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if !errors.Is(err, os.ErrClosed) {
			t.Errorf("concurrent operation failed: %v", err)
		}
	}
	want := map[string]bool{"initial": true}
	for message := range results {
		want[message] = true
	}
	for _, event := range readEvents(t, log.Path()) {
		if !want[event] {
			t.Errorf("unexpected, duplicated or interleaved event: %q", event)
		}
		delete(want, event)
	}
	if len(want) != 0 {
		t.Errorf("successful writes missing from journal: %v", want)
	}
}

func TestSecuresExistingJournal(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	if err := os.Mkdir(logs, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(logs, "arcana-world.log")
	if err := os.WriteFile(path, []byte("existing content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	log, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	for path, mode := range map[string]os.FileMode{logs: 0700, log.Path(): 0600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != mode {
			t.Errorf("%s permissions: %o, want %o", path, info.Mode().Perm(), mode)
		}
	}
	data, err := os.ReadFile(log.Path())
	if err != nil || string(data) != "existing content\n" {
		t.Fatalf("opening changed existing content: %q, %v", data, err)
	}
}

func TestRejectsUnsafeTargets(t *testing.T) {
	for _, target := range []string{"data symlink", "directory symlink", "file symlink", "nonregular file"} {
		t.Run(target, func(t *testing.T) {
			dir := t.TempDir()
			outside := t.TempDir()
			logs := filepath.Join(dir, "logs")
			if err := os.Mkdir(logs, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(logs, "arcana-world.log")
			victim := filepath.Join(outside, "victim")
			if err := os.WriteFile(victim, []byte("untouched"), 0600); err != nil {
				t.Fatal(err)
			}
			var err error
			switch target {
			case "data symlink":
				dir = filepath.Join(dir, "linked")
				err = os.Symlink(outside, dir)
			case "directory symlink":
				if err = os.Remove(logs); err == nil {
					err = os.Symlink(outside, logs)
				}
			case "file symlink":
				err = os.Symlink(victim, path)
			case "nonregular file":
				err = os.Mkdir(path, 0700)
			}
			if err != nil {
				t.Fatal(err)
			}
			if log, err := Open(dir); err == nil {
				_ = log.Close()
				t.Fatal("opened unsafe journal target")
			}
			data, err := os.ReadFile(victim)
			if err != nil || string(data) != "untouched" {
				t.Fatalf("symlink destination changed: %q, %v", data, err)
			}
		})
	}
}

func TestRetentionBoundaryAndContinuedWrites(t *testing.T) {
	dir := t.TempDir()
	log, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	now := time.Now().UTC().Truncate(time.Second)
	cutoff := now.Add(-retention)
	line := func(at time.Time, message string) string {
		return at.Format(time.RFC3339) + " " + message + "\n"
	}
	kept := line(cutoff, "boundary") + line(now, "new")
	// Expired records need not form a prefix after a clock adjustment.
	input := line(cutoff, "boundary") + line(cutoff.Add(-time.Second), "expired-private") + line(now, "new")
	if _, err := log.file.WriteString(input); err != nil {
		t.Fatal(err)
	}
	log.mu.Lock()
	err = log.prune(now)
	log.mu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(log.Path())
	if err != nil || string(data) != kept {
		t.Fatalf("retained log=%q err=%v", data, err)
	}
	if err := log.Write("after cleanup"); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, log.Path())
	if len(events) != 3 || events[2] != "after cleanup" {
		t.Fatalf("write targeted replaced inode: %q", events)
	}
	if info, err := os.Stat(log.Path()); err != nil || runtime.GOOS != "windows" && info.Mode().Perm() != 0600 {
		t.Fatalf("replacement permissions: %v %v", info, err)
	}
}

func TestRetentionStartupAndExclusiveWriter(t *testing.T) {
	dir := t.TempDir()
	log, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	path := log.Path()
	if _, err := Open(dir); err == nil {
		t.Fatal("second writer could race log replacement")
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-retention-time.Hour).Format(time.RFC3339) + " expired-private\n"
	newLine := time.Now().Format(time.RFC3339) + " retained\n"
	// Preserve unrecognized or partial records instead of silently discarding
	// data for which no trustworthy expiration timestamp is available.
	input := old + newLine + "legacy unknown timestamp\npartial"
	if err := os.WriteFile(path, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	abandoned := filepath.Join(filepath.Dir(path), ".journal-interrupted")
	if err := os.WriteFile(abandoned, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	log, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = log.Close() })
	data, err := os.ReadFile(path)
	if err != nil || string(data) != newLine+"legacy unknown timestamp\npartial" {
		t.Fatalf("startup retention=%q err=%v", data, err)
	}
	if _, err := os.Stat(abandoned); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("abandoned private copy survived: %v", err)
	}
}
