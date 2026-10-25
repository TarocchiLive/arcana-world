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
	logger := Logger("test")
	AddSecrets("session-cookie-value")
	logger.Info("first event", "event_id", 1, "err", errors.New("request rejected for session-cookie-value"))
	// 关闭前即可读到已脱敏的记录，不依赖字段顺序、源码位置或固定文案。
	events := readEvents(t, log.Path())
	if len(events) != 1 || !strings.Contains(events[0], "event_id=1") ||
		strings.Contains(events[0], "session-cookie-value") || !strings.Contains(events[0], "[REDACTED]") {
		t.Fatalf("events before close: %q", events)
	}
	first := events[0]
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	logger.Info("after close")
	log, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("second event", "event_id", 2)
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	events = readEvents(t, log.Path())
	if len(events) != 2 || events[0] != first || !strings.Contains(events[1], "event_id=2") {
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
	logger := Logger("test")
	logger.Info("still usable after contention", "event_id", 1)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	logger.Info("reopened", "event_id", 2)
	events := readEvents(t, reopened.Path())
	if len(events) != 2 || !strings.Contains(events[0], "event_id=1") || !strings.Contains(events[1], "event_id=2") {
		t.Fatalf("contention or reopening changed events: %q", events)
	}
}
