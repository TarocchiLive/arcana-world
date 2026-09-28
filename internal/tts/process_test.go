package tts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

const helperMode = "ARCANA_TTS_TEST_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(helperMode) == "wait" {
		_, _ = io.Copy(io.Discard, os.Stdin)
		conn, err := net.Dial("tcp", os.Getenv("ARCANA_TTS_TEST_READY"))
		if err != nil {
			os.Exit(2)
		}
		defer conn.Close()
		_, _ = io.Copy(io.Discard, conn)
		os.Exit(0)
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
				_, err := runHelper(ctx, executable, nil, []byte("request"), 8, append(fixtureEnv("wait"), "ARCANA_TTS_TEST_READY="+listener.Addr().String()))
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
		})
	}
}
