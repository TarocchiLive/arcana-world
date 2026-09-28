package overlay

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServeControlEOFCancelsNative(t *testing.T) {
	t.Setenv(tokenEnvironment, "test-secret")
	directory, err := privateSocketDir("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Error(err)
		}
	})
	socket := filepath.Join(directory, "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	stopListener := context.AfterFunc(ctx, func() { _ = listener.Close() })
	defer stopListener()
	hostDone := make(chan error, 1)
	go func() {
		hostDone <- Serve(ctx, socket, func(nativeCtx context.Context, cfg Config, updates <-chan Config, ready func()) error {
			ready()
			<-nativeCtx.Done()
			return nil
		})
	}()
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err = conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	hello, err := readFrame(conn)
	if err != nil {
		t.Fatal(err)
	}
	if hello.Type != "hello" || hello.Token != "test-secret" {
		t.Fatal("host did not authenticate")
	}
	cfg := DefaultConfig()
	cfg.Text = "initial native state"
	if err = writeFrame(conn, wireFrame{Type: "config", Config: &cfg}); err != nil {
		t.Fatal(err)
	}
	ready, err := readFrame(conn)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Type != "ready" {
		t.Fatalf("unexpected readiness frame %q", ready.Type)
	}
	if err = conn.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-hostDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("control EOF did not stop native host")
	}
}
