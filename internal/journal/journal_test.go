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
	logger.Info("first event", "err", errors.New("request rejected for session-cookie-value"))
	// 关闭前读取也必须能看到事件：日志并非仅在退出时才刷新缓冲。
	first := `level=INFO msg="first event" component=test err="request rejected for [REDACTED]"`
	if events := readEvents(t, log.Path()); len(events) != 1 || events[0] != first {
		t.Fatalf("events before close: %q", events)
	}
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
	logger.Info("second event")
	if err := log.Close(); err != nil {
		t.Fatal(err)
	}
	events := readEvents(t, log.Path())
	if len(events) != 2 || events[0] != first || events[1] != `level=INFO msg="second event" component=test` {
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
	logger.Info("still usable after contention")
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	logger.Info("reopened")
	events := readEvents(t, reopened.Path())
	if len(events) != 2 || events[0] != `level=INFO msg="still usable after contention" component=test` || events[1] != "level=INFO msg=reopened component=test" {
		t.Fatalf("contention or reopening changed events: %q", events)
	}
}
