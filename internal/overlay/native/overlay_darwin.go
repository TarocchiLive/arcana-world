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
	"encoding/json"
	"errors"
	"unsafe"

	"arcana-world/internal/overlay"
)

func darwinConfig(cfg overlay.Config, apply func(C.AWOverlayConfig) C.int) C.int {
	selected, err := cfg.SelectedDisplays()
	if err != nil {
		return 0
	}
	var displays *C.char
	if selected != nil {
		encoded, err := json.Marshal(selected)
		if err != nil {
			return 0
		}
		displays = C.CString(string(encoded))
		defer C.free(unsafe.Pointer(displays))
	}
	text, family := C.CString(cfg.Text), C.CString(cfg.Font.Family)
	defer C.free(unsafe.Pointer(text))
	defer C.free(unsafe.Pointer(family))
	textRGB, err := overlay.ParseColor(cfg.Colors.Text)
	if err != nil {
		return 0
	}
	backgroundRGB, err := overlay.ParseColor(cfg.Colors.Background)
	if err != nil {
		return 0
	}
	runs := cfg.TextRuns()
	var nativeRuns *C.AWOverlayTextRun
	if len(runs) != 0 {
		nativeRuns = (*C.AWOverlayTextRun)(C.calloc(C.size_t(len(runs)), C.size_t(C.sizeof_AWOverlayTextRun)))
		if nativeRuns == nil {
			return 0
		}
		defer C.free(unsafe.Pointer(nativeRuns))
		for i, run := range runs {
			unsafe.Slice(nativeRuns, len(runs))[i] = C.AWOverlayTextRun{
				start: C.size_t(run.Start), end: C.size_t(run.End), rgb: C.uint32_t(run.RGB),
			}
		}
	}
	italic := C.int(0)
	if cfg.Font.Italic {
		italic = 1
	}
	outline := C.int(0)
	if cfg.Outline {
		outline = 1
	}
	return apply(C.AWOverlayConfig{
		displays:    displays,
		text_length: C.size_t(len(cfg.Text)), runs: nativeRuns, run_count: C.size_t(len(runs)),
		text_rgb: C.uint32_t(textRGB), background_rgb: C.uint32_t(backgroundRGB),
		text: text, family: family, x: C.double(cfg.Position.X), y: C.double(cfg.Position.Y),
		width: C.double(cfg.Width), height: C.double(cfg.Height),
		top: C.double(cfg.Padding.Top), right: C.double(cfg.Padding.Right), bottom: C.double(cfg.Padding.Bottom), left: C.double(cfg.Padding.Left),
		font_size: C.double(cfg.Font.Size), weight: C.int(cfg.Font.Weight), italic: italic,
		text_alpha: C.double(cfg.TextAlpha), background_alpha: C.double(cfg.BackgroundAlpha),
		outline: outline,
		anchor:  C.int(cfg.Position.Anchor.Index()), display: C.uint(cfg.DisplayID),
	})
}

func runPlatform(ctx context.Context, cfg overlay.Config, updates <-chan overlay.Config, ready func()) error {
	if _, err := cfg.SelectedDisplays(); err != nil {
		return err
	}
	if cfg.Displays == "" && cfg.Output != "" {
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
				if updateErr == nil {
					_, updateErr = value.SelectedDisplays()
				}
				if updateErr == nil && value.Displays == "" && value.Output != "" {
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
					updateErr = errors.New("overlay: cannot apply macOS overlay configuration")
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

func listDisplaysPlatform(ctx context.Context, cfg overlay.Config) ([]overlay.Display, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	selected, err := cfg.SelectedDisplays()
	if err != nil {
		return nil, err
	}
	if cfg.Displays == "" && cfg.Output != "" {
		return nil, errors.New("overlay: macOS requires a display ID, not an output name")
	}
	value := C.overlay_list_displays(C.uint(cfg.DisplayID))
	if value == nil {
		return nil, errors.New("overlay: cannot enumerate macOS displays (requires a graphical session)")
	}
	defer C.free(unsafe.Pointer(value))
	var displays []overlay.Display
	if err := json.Unmarshal([]byte(C.GoString(value)), &displays); err != nil {
		return nil, err
	}
	if selected != nil {
		wanted := make(map[string]bool, len(selected))
		for _, id := range selected {
			wanted[id] = true
		}
		for i := range displays {
			displays[i].Selected = wanted[displays[i].ID]
		}
	}
	return displays, ctx.Err()
}
