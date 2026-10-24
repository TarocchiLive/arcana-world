package overlay

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
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
		hostDone <- Serve(ctx, socket, BackendNative, func(nativeCtx context.Context, cfg Config, updates <-chan Config, ready func()) error {
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
	if hello.Type != "hello" || hello.Token != "test-secret" || hello.Backend != BackendNative || hello.Capabilities != hostCapabilities {
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

func TestSettingsBackendDefaultsAndIsolation(t *testing.T) {
	var saved Settings
	if err := json.Unmarshal([]byte(`{"enabled":true,"text_alpha":0,"outline":false}`), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Backend != BackendNative || saved.Qt != DefaultQtAppearance() || saved.Qt.ShowInTaskbar || saved.TextAlpha != 0 || saved.Outline {
		t.Fatalf("legacy settings lost defaults or explicit values: %+v", saved)
	}
	if err := json.Unmarshal([]byte(`{"backend":"qt","qt":{"show_in_taskbar":true,"gradient":false,"animation_enabled":false,"animation_duration_ms":0,"history_alpha":0,"line_spacing":0,"border_alpha":0,"shadow_strength":0}}`), &saved); err != nil {
		t.Fatal(err)
	}
	appearance := saved.Qt
	if !appearance.ShowInTaskbar || appearance.Gradient || appearance.AnimationEnabled || appearance.AnimationDurationMS != 0 || appearance.HistoryAlpha != 0 || appearance.LineSpacing != 0 || appearance.BorderAlpha != 0 || appearance.ShadowStrength != 0 {
		t.Fatalf("explicit appearance values replaced with defaults: %+v", appearance)
	}
	for _, backend := range []Backend{BackendNative, BackendQt} {
		saved.Backend = backend
		encoded, err := json.Marshal(saved)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &saved); err != nil {
			t.Fatal(err)
		}
		if saved.Backend != backend || saved.Qt != appearance {
			t.Fatal("switching backend lost its persisted appearance")
		}
		cfg := saved.Config("")
		encoded, err = json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var decoded Config
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded.Qt != appearance {
			t.Fatal("config round trip lost its persisted appearance")
		}
	}
	for _, custom := range []bool{false, true} {
		previous := DefaultSettings()
		previous.Qt.HistoryScale, previous.Qt.LatestScale = .9, 1.05
		previous.Qt.HistoryAlpha, previous.Qt.Gradient = .55, true
		previous.Qt.ShowInTaskbar = true
		previous.Qt.AnimationDurationMS = 640
		if custom {
			previous.Qt.HistoryAlpha = .8
		}
		encoded, err := json.Marshal(previous)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(encoded, &saved); err != nil {
			t.Fatal(err)
		}
		expected := previous
		if !custom {
			expected.Qt.HistoryScale, expected.Qt.LatestScale = 1, 1
			expected.Qt.HistoryAlpha, expected.Qt.Gradient = .95, false
		}
		if !reflect.DeepEqual(saved, expected) {
			t.Fatalf("appearance migration changed unrelated or custom settings: got %+v, want %+v", saved, expected)
		}
	}
	if err := json.Unmarshal([]byte(`{"backend":"unknown"}`), &saved); err == nil {
		t.Fatal("accepted unknown backend")
	}
}
