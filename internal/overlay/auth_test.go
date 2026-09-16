package overlay

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestUnauthenticatedPeerDoesNotConsumeHostSlot(t *testing.T) {
	directory, err := privateSocketDir("")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(directory)
	socket := filepath.Join(directory, "s")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socket, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stop()
	result := make(chan error, 1)
	go func() {
		conn, err := acceptHost(ctx, listener, "correct-secret")
		if conn != nil {
			_ = conn.Close()
		}
		result <- err
	}()
	bad, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	if err = writeFrame(bad, wireFrame{Type: "hello", Token: "wrong-secret"}); err != nil {
		t.Fatal(err)
	}
	if err = bad.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var data [1]byte
	if _, err = bad.Read(data[:]); err == nil {
		t.Fatal("unauthenticated peer was not closed")
	}
	good, err := net.Dial("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer good.Close()
	if err = writeFrame(good, wireFrame{Type: "hello", Token: "correct-secret"}); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if err != nil {
			t.Fatalf("legitimate host rejected after invalid peer: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("legitimate host could not authenticate")
	}
}
