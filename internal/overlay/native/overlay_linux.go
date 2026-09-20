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
	"strings"
	"unsafe"

	"arcana-world/internal/overlay"
)

//export arcanaWaylandReady
func arcanaWaylandReady(handle C.uintptr_t) {
	cgo.Handle(handle).Value().(func())()
}

//export arcanaWaylandDisplay
func arcanaWaylandDisplay(handle C.uintptr_t, id, name *C.char) {
	displays := cgo.Handle(handle).Value().(*[]overlay.Display)
	*displays = append(*displays, overlay.Display{ID: C.GoString(id), Name: C.GoString(name)})
}

// 临时 C 字符串只在同步深复制期间存活，不把 Go 指针留给原生线程。
func waylandConfig(cfg overlay.Config) (C.struct_arcana_wayland_config, func(), error) {
	selected, err := cfg.SelectedDisplays()
	if err != nil {
		return C.struct_arcana_wayland_config{}, nil, err
	}
	for _, id := range selected {
		if strings.ContainsRune(id, '\x00') {
			return C.struct_arcana_wayland_config{}, nil, errors.New("Wayland display IDs cannot contain NUL")
		}
	}
	value := C.struct_arcana_wayland_config{
		text: C.CString(cfg.Text), output: C.CString(cfg.Output), family: C.CString(cfg.Font.Family),
		anchor: C.int(cfg.Position.Anchor.Index()), weight: C.int(cfg.Font.Weight),
		x: C.double(cfg.Position.X), y: C.double(cfg.Position.Y), width: C.double(cfg.Width), height: C.double(cfg.Height),
		padding_top: C.double(cfg.Padding.Top), padding_right: C.double(cfg.Padding.Right), padding_bottom: C.double(cfg.Padding.Bottom), padding_left: C.double(cfg.Padding.Left),
		font_size: C.double(cfg.Font.Size), text_alpha: C.double(cfg.TextAlpha), background_alpha: C.double(cfg.BackgroundAlpha),
	}
	if selected != nil {
		value.explicit_displays = 1
		if len(selected) > 0 {
			packed := strings.Join(selected, "\x00") + "\x00"
			value.displays = C.CString(packed)
			value.displays_size = C.size_t(len(packed))
		}
	}
	if cfg.Font.Italic {
		value.italic = 1
	}
	return value, func() {
		C.free(unsafe.Pointer(value.text))
		C.free(unsafe.Pointer(value.output))
		C.free(unsafe.Pointer(value.family))
		C.free(unsafe.Pointer(value.displays))
	}, nil
}

func runPlatform(ctx context.Context, cfg overlay.Config, updates <-chan overlay.Config, ready func()) error {
	if cfg.Displays == "" && cfg.DisplayID != 0 {
		return errors.New("Wayland does not support display IDs; use a native output name")
	}
	handle := cgo.NewHandle(ready)
	defer handle.Delete()
	initial, release, err := waylandConfig(cfg)
	if err != nil {
		return err
	}
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
				if err == nil && normalized.Displays == "" && normalized.DisplayID != 0 {
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
				native, free, err := waylandConfig(normalized)
				if err != nil {
					producerErr = err
					C.arcana_wayland_stop(s)
					return
				}
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

func listDisplaysPlatform(ctx context.Context, cfg overlay.Config) ([]overlay.Display, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cfg.Displays == "" && cfg.DisplayID != 0 {
		return nil, errors.New("Wayland does not support display IDs; use a native output name")
	}
	initial, release, err := waylandConfig(cfg)
	if err != nil {
		return nil, err
	}
	s := C.arcana_wayland_new(&initial, 0)
	release()
	if s == nil {
		return nil, errors.New("Wayland enumeration: cannot allocate state or wakeup pipe")
	}
	defer C.arcana_wayland_free(s)
	displays := make([]overlay.Display, 0)
	handle := cgo.NewHandle(&displays)
	defer handle.Delete()
	done, joined := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
			C.arcana_wayland_stop(s)
		case <-done:
		}
	}()
	message := C.arcana_wayland_list(s, C.uintptr_t(handle))
	close(done)
	<-joined
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if message != nil {
		return nil, fmt.Errorf("Wayland enumeration: %s", C.GoString(message))
	}
	selected, err := cfg.SelectedDisplays()
	if err != nil {
		return nil, err
	}
	for i := range displays {
		if selected == nil {
			displays[i].Selected = displays[i].ID == cfg.Output || (cfg.Output == "" && i == 0)
			continue
		}
		for _, id := range selected {
			if displays[i].ID == id {
				displays[i].Selected = true
				break
			}
		}
	}
	return displays, nil
}
