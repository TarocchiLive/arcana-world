package journal

import (
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
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

func TestOpenWhileDirectoryOccupied(t *testing.T) {
	dir := t.TempDir()
	first, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second, err := Open(dir)
	if second != nil {
		_ = second.Close()
		t.Fatal("second open succeeded while the directory was occupied")
	}
	if !errors.Is(err, bolt.ErrTimeout) {
		t.Fatalf("lock contention did not retain its cause: %v", err)
	}
	if err := first.Write("still usable after contention"); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	if err := reopened.Write("reopened"); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, reopened.Path())
	if len(events) != 2 || events[0] != "still usable after contention" || events[1] != "reopened" {
		t.Fatalf("contention or reopening changed events: %q", events)
	}
}
