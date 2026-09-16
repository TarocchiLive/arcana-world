//go:build darwin && cgo

package native

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework CoreGraphics
#include <stdlib.h>
#include "panel_darwin.h"
*/
import "C"

import (
	"context"
	"errors"
	"unsafe"

	"arcana-world/internal/overlay"
)

func darwinConfig(cfg overlay.Config, apply func(C.AWOverlayConfig) C.int) C.int {
	text, family := C.CString(cfg.Text), C.CString(cfg.Font.Family)
	defer C.free(unsafe.Pointer(text))
	defer C.free(unsafe.Pointer(family))
	italic := C.int(0)
	if cfg.Font.Italic {
		italic = 1
	}
	return apply(C.AWOverlayConfig{
		text: text, family: family, x: C.double(cfg.Position.X), y: C.double(cfg.Position.Y),
		width: C.double(cfg.Width), height: C.double(cfg.Height),
		top: C.double(cfg.Padding.Top), right: C.double(cfg.Padding.Right), bottom: C.double(cfg.Padding.Bottom), left: C.double(cfg.Padding.Left),
		font_size: C.double(cfg.Font.Size), weight: C.int(cfg.Font.Weight), italic: italic,
		text_alpha: C.double(cfg.TextAlpha), background_alpha: C.double(cfg.BackgroundAlpha),
		anchor: C.int(cfg.Position.Anchor.Index()), display: C.uint(cfg.DisplayID),
	})
}

func runPlatform(ctx context.Context, cfg overlay.Config, updates <-chan overlay.Config, ready func()) error {
	if cfg.Output != "" {
		return errors.New("overlay: macOS requires a display ID, not an output name")
	}
	if darwinConfig(cfg, func(value C.AWOverlayConfig) C.int { return C.overlay_create(value) }) == 0 {
		return errors.New("overlay: cannot create macOS overlay (requires a graphical session, valid display ID and usable font)")
	}
	ready()
	// 原生循环自行退出时也要结束生产者，避免等待尚未取消的上下文。
	nativeDone := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		var updateErr error
		defer func() { done <- updateErr }()
		for {
			select {
			case <-nativeDone:
				return
			case <-ctx.Done():
				C.overlay_stop()
				return
			case value, open := <-updates:
				if !open {
					updates = nil
					continue
				}
				value, updateErr = value.Normalize()
				if updateErr == nil && value.Output != "" {
					updateErr = errors.New("overlay: macOS requires a display ID, not an output name")
				}
				if updateErr != nil {
					C.overlay_stop()
					return
				}
				if value == cfg {
					continue
				}
				if darwinConfig(value, func(state C.AWOverlayConfig) C.int { return C.overlay_update(state) }) == 0 {
					updateErr = errors.New("overlay: requested macOS display is unavailable")
					C.overlay_stop()
					return
				}
				cfg = value
			}
		}
	}()
	status := C.overlay_run()
	close(nativeDone)
	if err := <-done; err != nil {
		return err
	}
	if status == 0 {
		return errors.New("overlay: macOS could not apply the requested display or font")
	}
	return nil
}
