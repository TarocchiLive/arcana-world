package tui

import (
	"context"
	"errors"
	"testing"

	"arcana-world/internal/overlay"
)

func TestQueuedOverlayStartIsCanceledOnClose(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	if err := m.ConfigureOverlay(overlay.Options{Config: overlay.DefaultConfig()}, true); err != nil {
		t.Fatal(err)
	}
	start := m.startOverlay()
	if start == nil {
		t.Fatal("enabled overlay did not queue startup")
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	result := start().(overlayStartedMsg)
	if result.manager != nil || !errors.Is(result.err, context.Canceled) {
		t.Fatalf("queued overlay startup escaped shutdown: manager=%v, err=%v", result.manager, result.err)
	}
}
