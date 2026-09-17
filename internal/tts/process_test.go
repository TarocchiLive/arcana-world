package tts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const helperMode = "ARCANA_TTS_TEST_HELPER"

func TestMain(m *testing.M) {
	switch os.Getenv(helperMode) {
	case "diagnostic":
		fmt.Fprintln(os.Stderr, os.Getenv("ARCANA_TTS_TEST_DIAGNOSTIC"))
		os.Exit(1)
	case "resolve":
		path, err := resolveHelper("arcana-world-tts", "")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprint(os.Stdout, path)
		os.Exit(0)
	case "edge":
		if err := RunEdge(os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	case "proxy":
		proxy, err := effectiveProxy(os.Getenv("ARCANA_TTS_TEST_PROXY"))
		if err != nil {
			fmt.Fprintln(os.Stdout, err)
			os.Exit(0)
		}
		fmt.Fprint(os.Stdout, proxy)
		os.Exit(0)
	case "wait", "fail-ready":
		_, _ = io.Copy(io.Discard, os.Stdin)
		conn, err := net.Dial("tcp", os.Getenv("ARCANA_TTS_TEST_READY"))
		if err != nil {
			os.Exit(2)
		}
		if os.Getenv(helperMode) == "fail-ready" {
			conn.Close()
			os.Exit(7)
		}
		defer conn.Close()
		_, _ = io.Copy(io.Discard, conn)
		os.Exit(0)
	case "overflow":
		_, _ = os.Stdout.Write(bytes.Repeat([]byte{'x'}, 4096))
		for {
			_, _ = os.Stderr.Write(bytes.Repeat([]byte{'s'}, 4096))
		}
	case "stderr":
		_, _ = os.Stderr.Write(bytes.Repeat([]byte{'s'}, 1<<20))
		_, _ = os.Stdout.Write([]byte("ok"))
		os.Exit(0)
	case "fail":
		fmt.Fprintln(os.Stderr, "private-text proxy-password")
		os.Exit(7)
	case "empty":
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
			if _, err := conn.Read(b[:]); err != io.EOF {
				t.Fatalf("child connection still open: %v", err)
			}
		})
	}
}
func TestHelperOutputAndFailures(t *testing.T) {
	executable := testExecutable(t)
	for _, tc := range []struct {
		mode  string
		limit int
		want  string
		fail  bool
	}{
		{"overflow", 32, "", true}, {"stderr", 2, "ok", false}, {"stderr", 0, "", true}, {"fail", 8, "", true}, {"empty", 0, "", false},
	} {
		t.Run(tc.mode+fmt.Sprint(tc.limit), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			data, err := runHelper(ctx, executable, nil, nil, tc.limit, fixtureEnv(tc.mode))
			if (err != nil) != tc.fail || string(data) != tc.want {
				t.Fatalf("output=%q error=%v", data, err)
			}
			if errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("blocked while draining output")
			}
			if err != nil && (strings.Contains(err.Error(), "private-text") || strings.Contains(err.Error(), "proxy-password")) {
				t.Fatal("private diagnostics leaked")
			}
		})
	}
	edge := &Edge{executable: executable, voice: defaultVoice, timeout: 5 * time.Second}
	t.Setenv(helperMode, "empty")
	if _, err := edge.Synthesize(context.Background(), "hello"); err == nil || !strings.Contains(err.Error(), "empty audio") {
		t.Fatalf("empty synthesis: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runHelper(ctx, executable, nil, nil, 0, fixtureEnv("empty")); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := runHelper(context.Background(), filepath.Join(t.TempDir(), "missing"), nil, nil, 0, nil); err == nil {
		t.Fatal("missing child accepted")
	}
}
func TestResolveHelper(t *testing.T) {
	path := filepath.Join(t.TempDir(), "directory with spaces", "helper")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveHelper("arcana-world-tts", path); err == nil {
		t.Fatal("missing helper accepted")
	}
	if err := os.WriteFile(path, []byte("not run"), 0600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if _, err := resolveHelper("arcana-world-tts", path); err == nil {
			t.Fatal("nonexecutable helper accepted")
		}
	}
	if err := os.Chmod(path, 0700); err != nil {
		t.Fatal(err)
	}
	got, err := resolveHelper("arcana-world-tts", path)
	if err != nil || got != path {
		t.Fatalf("%q %v", got, err)
	}
	if _, err := resolveHelper("arcana-world-tts", filepath.Dir(path)); err == nil {
		t.Fatal("directory accepted")
	}
	// A same-name executable in the working directory must never be selected.
	t.Chdir(filepath.Dir(path))
	name := "arcana-world-tts"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	if err := os.WriteFile(name, []byte("not run"), 0700); err != nil {
		t.Fatal(err)
	}
	got, err = resolveHelper("arcana-world-tts", "")
	if err == nil && filepath.Dir(got) == filepath.Dir(path) {
		t.Fatal("selected working-directory executable")
	}
}

func TestRelocatedHelperUsesInstallationDirectory(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "portable directory")
	if err := os.MkdirAll(filepath.Join(directory, "libexec"), 0700); err != nil {
		t.Fatal(err)
	}
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	executable := filepath.Join(directory, "application"+suffix)
	source, err := os.Open(testExecutable(t))
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	target, err := os.OpenFile(executable, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0700)
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(target, source)
	closeErr := target.Close()
	if copyErr != nil {
		t.Fatal(copyErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	helper := filepath.Join(directory, "libexec", "arcana-world-tts"+suffix)
	if err := os.WriteFile(helper, []byte("helper"), 0700); err != nil {
		t.Fatal(err)
	}
	resolvedHelper, err := filepath.EvalSymlinks(helper)
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	launches := []string{executable}
	if runtime.GOOS != "windows" {
		launcher := filepath.Join(t.TempDir(), "launcher")
		if err := os.Symlink(executable, launcher); err != nil {
			t.Fatal(err)
		}
		launches = append(launches, launcher)
	}
	for _, launch := range launches {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		data, err := runHelper(ctx, launch, nil, nil, 4096, fixtureEnv("resolve"))
		cancel()
		if err != nil || string(data) != resolvedHelper {
			t.Fatalf("relocated resolution: %q %v", data, err)
		}
	}
	if err := os.Remove(helper); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := runHelper(ctx, executable, nil, nil, 4096, fixtureEnv("resolve")); err == nil {
		t.Fatal("missing bundled helper accepted")
	}
}

func TestHelperFailedExitRacingCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	executable := testExecutable(t)
	go func() {
		_, err := runHelper(ctx, executable, nil, nil, 0, append(fixtureEnv("fail-ready"), "ARCANA_TTS_TEST_READY="+listener.Addr().String()))
		result <- err
	}()
	listener.(*net.TCPListener).SetDeadline(time.Now().Add(5 * time.Second))
	conn, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var b [1]byte
	if _, err := conn.Read(b[:]); err != io.EOF {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("failed child reported successful playback")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("failed child not reaped")
	}
}

func TestHelperPreservesSafeFailureCategories(t *testing.T) {
	for _, tc := range []struct{ diagnostic, category string }{
		{"audio: MP3 sample rate must be 24000 Hz", "sample rate"},
		{"audio: initialize device: private-text proxy-password", "initialize device"},
		{"audio: device: private-text proxy-password", "device"},
		{"audio: player: private-text proxy-password", "player"},
		{"audio: decode PCM: private-text proxy-password", "decode PCM"},
		{"tts: synthesis handshake rejected", "handshake"},
	} {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := runHelper(ctx, testExecutable(t), nil, nil, 0, append(fixtureEnv("diagnostic"), "ARCANA_TTS_TEST_DIAGNOSTIC="+tc.diagnostic))
		cancel()
		if err == nil || !strings.Contains(err.Error(), tc.category) {
			t.Fatalf("lost failure category: %v", err)
		}
		if strings.Contains(err.Error(), "private-text") || strings.Contains(err.Error(), "proxy-password") {
			t.Fatalf("private diagnostic leaked: %v", err)
		}
	}
}
