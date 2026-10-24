package rtmpsrt

import (
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
)

type trackedCloser struct{ calls atomic.Int32 }

func (c *trackedCloser) Close() error { c.calls.Add(1); return nil }

func TestBridgeCloseReleasesListenerAndConnections(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	bridge := &Bridge{listener: listener, ctx: ctx, cancel: cancel, active: make(map[io.Closer]struct{})}
	connection := &trackedCloser{}
	if !bridge.track(connection) {
		t.Fatal("tracking live connection failed")
	}
	bridge.Close()
	bridge.Close()
	if ctx.Err() == nil || connection.calls.Load() != 1 {
		t.Fatal("Close did not cancel and close resources exactly once")
	}
	if _, err := listener.Accept(); err == nil {
		t.Fatal("listener still accepting")
	}
	late := &trackedCloser{}
	if bridge.track(late) || late.calls.Load() != 1 {
		t.Fatal("late connection escaped closed bridge")
	}
}
