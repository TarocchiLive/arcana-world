package overlay

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// 子进程运行真实 Unix socket 宿主；只替换图形循环，避免单元测试依赖桌面。
func TestMain(m *testing.M) {
	if len(os.Args) == 3 && os.Args[1] == "-socket" && os.Getenv("ARCANA_OVERLAY_TEST_MODE") != "" {
		if err := testHost(os.Args[2]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}
func testHost(socket string) error {
	marker := func() {
		if path := os.Getenv("ARCANA_OVERLAY_TEST_MARKER"); path != "" {
			_ = os.WriteFile(path, []byte("entered"), 0600)
		}
	}
	if os.Getenv("ARCANA_OVERLAY_TEST_MODE") == "stall-config" {
		conn, err := net.Dial("unix", socket)
		if err != nil {
			return err
		}
		defer conn.Close()
		if err = writeFrame(conn, wireFrame{Type: "hello", Token: os.Getenv(tokenEnvironment)}); err != nil {
			return err
		}
		marker()
		time.Sleep(time.Minute)
		return nil
	}
	return Serve(context.Background(), socket, func(ctx context.Context, _ Config, updates <-chan Config, ready func()) error {
		marker()
		if os.Getenv("ARCANA_OVERLAY_TEST_MODE") == "stall-ready" {
			<-ctx.Done()
			return nil
		}
		ready()
		for {
			select {
			case <-ctx.Done():
				return nil
			case cfg, ok := <-updates:
				if !ok || cfg.Text == "stop" {
					return nil
				}
			}
		}
	})
}
func helperOptions(t *testing.T, mode string) Options {
	t.Helper()
	t.Setenv("ARCANA_OVERLAY_TEST_MODE", mode)
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	root, err := privateSocketDir("")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return Options{Executable: executable, Config: DefaultConfig(), StartupTimeout: 5 * time.Second, ShutdownTimeout: 500 * time.Millisecond, RuntimeDir: root}
}
func assertRuntimeClean(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("socket directory remains after child exit: %v", entries)
	}
}
func TestManagerNormalNativeStopAndConcurrentClose(t *testing.T) {
	options := helperOptions(t, "stop")
	manager, err := Start(context.Background(), options)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	if err = manager.SetText("stop"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-manager.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("normal native exit did not stop manager")
	}
	if err = manager.Err(); err != nil {
		t.Fatalf("normal native exit reported as failure: %v", err)
	}
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := manager.Close(); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	if err = manager.SetText("after close"); err == nil {
		t.Fatal("accepted update after close")
	}
	assertRuntimeClean(t, options.RuntimeDir)
}
func TestManagerStartupDeadlineInterruptsConfigWrite(t *testing.T) {
	options := helperOptions(t, "stall-config")
	options.StartupTimeout = time.Second
	options.Config.Text = strings.Repeat("\x01", MaxTextBytes)
	marker := filepath.Join(t.TempDir(), "host-entered")
	t.Setenv("ARCANA_OVERLAY_TEST_MARKER", marker)
	start := time.Now()
	manager, err := Start(context.Background(), options)
	if manager != nil {
		_ = manager.Close()
		t.Fatal("nonreading child became ready")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("startup error=%v", err)
	}
	if _, err = os.Stat(marker); err != nil {
		t.Fatalf("helper did not reach blocked-write scenario: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 1800*time.Millisecond {
		t.Fatalf("frame write extended startup deadline: %s", elapsed)
	}
	assertRuntimeClean(t, options.RuntimeDir)
}
func TestManagerCancellationBeforeNativeReady(t *testing.T) {
	options := helperOptions(t, "stall-ready")
	marker := filepath.Join(t.TempDir(), "host-entered")
	t.Setenv("ARCANA_OVERLAY_TEST_MARKER", marker)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		manager, err := Start(ctx, options)
		if manager != nil {
			_ = manager.Close()
		}
		finished <- err
	}()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("host did not enter native initialization")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation became transport error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("startup cancellation did not reap child")
	}
	assertRuntimeClean(t, options.RuntimeDir)
}
