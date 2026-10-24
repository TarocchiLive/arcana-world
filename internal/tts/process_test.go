package tts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"arcana-world/internal/tts/meloproto"
)

const helperMode = "ARCANA_TTS_TEST_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(helperMode) == "resident" {
		if err := meloproto.Write(os.Stdout, []byte("test-ready")); err != nil {
			os.Exit(2)
		}
		for {
			input, err := meloproto.Read(os.Stdin, 64)
			if errors.Is(err, io.EOF) {
				os.Exit(0)
			}
			if err != nil {
				os.Exit(2)
			}
			if string(input) == "wait" {
				conn, err := net.Dial("tcp", os.Getenv("ARCANA_TTS_TEST_READY"))
				if err != nil {
					os.Exit(2)
				}
				_, _ = io.Copy(io.Discard, conn)
				_ = conn.Close()
				os.Exit(0)
			}
			if err := meloproto.Write(os.Stdout, []byte(fmt.Sprint(os.Getpid()))); err != nil {
				os.Exit(2)
			}
		}
	}
	os.Exit(m.Run())
}

func testExecutable(t *testing.T) string {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return executable
}
func fixtureEnv(mode string) []string {
	return append(withoutTestEnv(os.Environ()), helperMode+"="+mode, "GORACE=atexit_sleep_ms=0")
}
func withoutTestEnv(env []string) []string {
	result := make([]string, 0, len(env))
	for _, entry := range env {
		if !strings.HasPrefix(entry, helperMode+"=") {
			result = append(result, entry)
		}
	}
	return result
}

func TestHelperCancellationReapsChild(t *testing.T) {
	executable := testExecutable(t)
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint("deadline=", deadline), func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			client := newResidentClient()
			t.Cleanup(func() { _ = client.Close() })
			start := func(context.Context) (*exec.Cmd, error) {
				cmd := exec.Command(executable)
				cmd.Env = append(fixtureEnv("resident"), "ARCANA_TTS_TEST_READY="+listener.Addr().String())
				return cmd, nil
			}
			request := func(ctx context.Context, text string) ([]byte, error) {
				return client.request(ctx, start, "test-ready", []byte(text), 64, nil)
			}
			first, err := request(context.Background(), "pid")
			if err != nil {
				t.Fatal(err)
			}
			worker := client.worker
			select {
			case <-worker.done:
				t.Fatal("idle worker exited")
			case <-time.After(20 * time.Millisecond):
			}
			second, err := request(context.Background(), "pid")
			if err != nil || string(first) != string(second) {
				t.Fatalf("worker not reused: %q %q %v", first, second, err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			want := context.Canceled
			if deadline {
				cancel()
				ctx, cancel = context.WithTimeout(context.Background(), 2*time.Second)
				want = context.DeadlineExceeded
			}
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := request(ctx, "wait")
				result <- err
			}()
			listener.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
			conn, err := listener.Accept()
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if !deadline {
				cancel()
			}
			select {
			case err := <-result:
				if !errors.Is(err, want) {
					t.Fatalf("got %v, want %v", err, want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("child not reaped")
			}
			conn.SetReadDeadline(time.Now().Add(time.Second))
			var b [1]byte
			// 终止进程可能重置 TCP 连接，而不是发送 FIN。
			// Winsock 的 WSAECONNRESET 与 Go 的通用 ECONNRESET 错误码不同。
			const wsaeconnreset = syscall.Errno(10054)
			n, err := conn.Read(b[:])
			reset := errors.Is(err, syscall.ECONNRESET) ||
				runtime.GOOS == "windows" && errors.Is(err, wsaeconnreset)
			if n != 0 || !errors.Is(err, io.EOF) && !reset {
				t.Fatalf("child connection still open: n=%d err=%v", n, err)
			}
			select {
			case <-worker.done:
			default:
				t.Fatal("cancelled worker not reaped")
			}
			if _, err := request(context.Background(), "pid"); err != nil {
				t.Fatalf("restart after cancellation: %v", err)
			}
			worker = client.worker
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case <-worker.done:
			default:
				t.Fatal("idle worker not reaped by Close")
			}
			if _, err := request(context.Background(), "pid"); !errors.Is(err, ErrClosed) {
				t.Fatalf("request after Close: %v", err)
			}
		})
	}
}
