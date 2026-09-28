package danmaku

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"arcana-world/internal/domain"
)

type sourceFunc struct {
	room   int64
	listen func(context.Context, int64, int, func(), func(json.RawMessage) error) error
}

func (s sourceFunc) Room(context.Context) (domain.Room, error) { return domain.Room{ID: s.room}, nil }
func (s sourceFunc) ListenDanmaku(ctx context.Context, room int64, attempt int, ready func(), receive func(json.RawMessage) error) error {
	return s.listen(ctx, room, attempt, ready, receive)
}
func waitSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("listener did not make progress")
	}
}
func waitPhase(t *testing.T, l *Listener, phase Phase) {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if l.Snapshot().Phase == phase {
			return
		}
		select {
		case <-deadline.C:
			t.Fatalf("expected phase %s, got %+v", phase, l.Snapshot())
		case <-tick.C:
		}
	}
}
func listenerHistory(t *testing.T) *History {
	t.Helper()
	h, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return h
}
func message(id, text string) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"cmd":"DANMU_MSG","info":[[0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,{"extra":"{\"id_str\":\"%s\"}"}],%q,[1,"sender"]]}`, id, text))
}

func TestListenerIdempotentConfigureDrainsOldSession(t *testing.T) {
	h := listenerHistory(t)
	first, second, stopping, release := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	var active, calls atomic.Int32
	l := newListener(context.Background(), h, func(_ string, a domain.Account) (source, error) {
		room, _ := strconv.ParseInt(a.UID, 10, 64)
		return sourceFunc{room: room, listen: func(ctx context.Context, _ int64, _ int, ready func(), receive func(json.RawMessage) error) error {
			if active.Add(1) != 1 {
				t.Error("overlapping account sessions")
			}
			defer active.Add(-1)
			calls.Add(1)
			ready()
			if room == 1 {
				close(first)
				<-ctx.Done()
				close(stopping)
				<-release
				return receive(message("last-old", "saved before switching"))
			}
			close(second)
			<-ctx.Done()
			return ctx.Err()
		}}, nil
	})
	defer l.Close()
	a := domain.Account{UID: "1", Cookies: map[string]string{"SESSDATA": "one"}}
	l.Configure(&a, "direct", true)
	waitSignal(t, first)
	firstSnapshot := l.Snapshot()
	for range 20 {
		l.Configure(&a, "direct", true)
	}
	a.UID = "2"
	l.Configure(&a, "direct", true)
	switching := l.Snapshot()
	if switching.AccountUID != "2" || switching.RoomID != 0 || switching.Generation == firstSnapshot.Generation {
		t.Errorf("account switch exposed an old room: %+v", switching)
	}
	waitSignal(t, stopping)
	select {
	case <-second:
		t.Fatal("new account started before old reader drained")
	default:
	}
	close(release)
	waitSignal(t, second)
	current := l.Snapshot()
	if current.AccountUID != "2" || current.RoomID != 2 || current.Generation != switching.Generation {
		t.Errorf("new session published the wrong source: %+v", current)
	}
	if calls.Load() != 2 {
		t.Fatalf("same configuration restarted sessions: %d", calls.Load())
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	records, err := h.Page(1, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range records {
		if e.Text == "saved before switching" {
			found = true
		}
	}
	if !found {
		t.Fatal("completed old-session payload was lost on account change")
	}
}

type failingArchive struct {
	h    *History
	fail atomic.Bool
}

var errDisk = errors.New("injected disk failure")

func (f *failingArchive) Append(room int64, raw json.RawMessage) (bool, error) {
	if f.fail.Load() {
		return false, errDisk
	}
	return f.h.Append(room, raw)
}
func (f *failingArchive) RecordGap(room int64, text string) error { return f.h.RecordGap(room, text) }

func TestListenerRetainsFailedWriteAcrossAccountSwitch(t *testing.T) {
	h := listenerHistory(t)
	disk := &failingArchive{h: h}
	disk.fail.Store(true)
	second := make(chan struct{})
	drained := make(chan struct{})
	l := newListener(context.Background(), disk, func(_ string, a domain.Account) (source, error) {
		room, _ := strconv.ParseInt(a.UID, 10, 64)
		return sourceFunc{room: room, listen: func(ctx context.Context, _ int64, _ int, ready func(), receive func(json.RawMessage) error) error {
			ready()
			if room == 1 {
				defer close(drained)
				// 两条消息已从同一帧解码。它们都没有
				// ID，因此意外重放不会被去重掩盖。
				if err := receive(message("", "retained while disk unavailable")); err != nil {
					return err
				}
				return receive(message("", "remaining message in completed frame"))
			}
			close(second)
			<-ctx.Done()
			return ctx.Err()
		}}, nil
	})
	l.retryDelay = time.Millisecond
	defer l.Close()
	l.Configure(&domain.Account{UID: "1"}, "direct", true)
	waitPhase(t, l, "storage_error")
	l.Configure(&domain.Account{UID: "2"}, "direct", true)
	waitSignal(t, drained)
	// 磁盘恢复前发生取消，也必须保留整个已完成的帧。
	disk.fail.Store(false)
	waitSignal(t, second)
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	records, _ := h.Page(1, 0, 100)
	counts := map[string]int{}
	for _, e := range records {
		if e.Kind == "chat" {
			counts[e.Text]++
		}
	}
	if counts["retained while disk unavailable"] != 1 || counts["remaining message in completed frame"] != 1 {
		t.Fatalf("pending frame lost or duplicated on target change: %v", counts)
	}
}
