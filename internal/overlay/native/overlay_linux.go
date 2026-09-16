//go:build linux && cgo && wayland

package native

/*
#cgo pkg-config: wayland-client pangocairo
#cgo LDFLAGS: -lm -lpthread
#include <stdlib.h>
#include "wayland_linux.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"runtime/cgo"
	"unsafe"

	"arcana-world/internal/overlay"
)

//export arcanaWaylandReady
func arcanaWaylandReady(handle C.uintptr_t) {
	cgo.Handle(handle).Value().(func())()
}

// 临时 C 字符串只在同步深复制期间存活，不把 Go 指针留给原生线程。
func waylandConfig(cfg overlay.Config) (C.struct_arcana_wayland_config, func()) {
	value := C.struct_arcana_wayland_config{
		text: C.CString(cfg.Text), output: C.CString(cfg.Output), family: C.CString(cfg.Font.Family),
		anchor: C.int(cfg.Position.Anchor.Index()), weight: C.int(cfg.Font.Weight),
		x: C.double(cfg.Position.X), y: C.double(cfg.Position.Y), width: C.double(cfg.Width), height: C.double(cfg.Height),
		padding_top: C.double(cfg.Padding.Top), padding_right: C.double(cfg.Padding.Right), padding_bottom: C.double(cfg.Padding.Bottom), padding_left: C.double(cfg.Padding.Left),
		font_size: C.double(cfg.Font.Size), text_alpha: C.double(cfg.TextAlpha), background_alpha: C.double(cfg.BackgroundAlpha),
	}
	if cfg.Font.Italic {
		value.italic = 1
	}
	return value, func() {
		C.free(unsafe.Pointer(value.text))
		C.free(unsafe.Pointer(value.output))
		C.free(unsafe.Pointer(value.family))
	}
}

func runPlatform(ctx context.Context, cfg overlay.Config, updates <-chan overlay.Config, ready func()) error {
	if cfg.DisplayID != 0 {
		return errors.New("Wayland does not support display IDs; use a native output name")
	}
	handle := cgo.NewHandle(ready)
	defer handle.Delete()
	initial, release := waylandConfig(cfg)
	s := C.arcana_wayland_new(&initial, C.uintptr_t(handle))
	release()
	if s == nil {
		return errors.New("Wayland overlay: cannot allocate state or wakeup pipe")
	}
	defer C.arcana_wayland_free(s)
	// 释放状态和回调句柄前先等待生产者退出，包括原生启动失败的路径。
	done, joined := make(chan struct{}), make(chan struct{})
	var producerErr error
	go func() {
		defer close(joined)
		previous := cfg
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				C.arcana_wayland_stop(s)
				return
			case value, ok := <-updates:
				if !ok {
					updates = nil
					continue
				}
				normalized, err := value.Normalize()
				if err == nil && normalized.DisplayID != 0 {
					err = errors.New("Wayland does not support display IDs; use a native output name")
				}
				if err != nil {
					producerErr = err
					C.arcana_wayland_stop(s)
					return
				}
				if normalized == previous {
					continue
				}
				native, free := waylandConfig(normalized)
				accepted := C.arcana_wayland_update(s, &native)
				free()
				if accepted == 0 {
					producerErr = errors.New("Wayland overlay: cannot allocate full-state update")
					C.arcana_wayland_stop(s)
					return
				}
				previous = normalized
			}
		}
	}()
	message := C.arcana_wayland_run(s)
	close(done)
	<-joined
	if producerErr != nil {
		return producerErr
	}
	if ctx.Err() != nil {
		return nil
	}
	if message != nil {
		return fmt.Errorf("Wayland overlay: %s", C.GoString(message))
	}
	return nil
}
