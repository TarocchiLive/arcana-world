//go:build qt && cgo

// Package qt 在主 OS 线程运行可选的 Qt Widgets 浮层。
package qt

/*
#cgo LDFLAGS: -lawqt
#include <stdlib.h>
#include "cpp/bridge.h"
*/
import "C"

import (
	"context"
	"encoding/json"
	"errors"
	"runtime"
	"sync/atomic"
	"unsafe"

	"arcana-world/internal/overlay"
)

var started atomic.Bool
var options struct {
	ReducedMotion bool `json:"reduced_motion"`
	Unlocked      bool `json:"unlocked"`
}

// Configure 必须在 Run 前由主线程调用；运行期间设置不变。
func Configure(reducedMotion, unlocked bool) {
	options.ReducedMotion = reducedMotion
	options.Unlocked = unlocked
}

func encode(cfg overlay.Config) (*C.char, error) {
	cfg, err := cfg.Normalize()
	if err != nil {
		return nil, err
	}
	if cfg.Displays == "" {
		if runtime.GOOS == "darwin" && cfg.Output != "" {
			return nil, errors.New("overlay: macOS requires a display ID, not an output name")
		}
		if runtime.GOOS == "linux" && cfg.DisplayID != 0 {
			return nil, errors.New("overlay: Linux requires an output name, not a display ID")
		}
	}
	value := struct {
		overlay.Config
		ReducedMotion bool `json:"reduced_motion"`
		Unlocked      bool `json:"unlocked"`
	}{cfg, options.ReducedMotion, options.Unlocked}
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return C.CString(string(data)), nil
}

func bridgeError() error {
	message := C.GoString(C.awqt_error())
	if message == "" {
		message = "Elegant overlay operation failed"
	}
	return errors.New(message)
}

// Run 每进程仅运行一次；调用者必须锁定主 OS 线程。
func Run(ctx context.Context, cfg overlay.Config, updates <-chan overlay.Config, ready func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	value, err := encode(cfg)
	if err != nil {
		return err
	}
	defer C.free(unsafe.Pointer(value))
	if !started.CompareAndSwap(false, true) {
		return errors.New("Elegant overlay: host may run only once per process")
	}
	if C.awqt_create(value) == 0 {
		return bridgeError()
	}
	if ready != nil {
		ready()
	}
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
				C.awqt_stop()
				return
			case next, open := <-updates:
				if !open {
					// 输入结束后保留最后一帧，直到上下文取消。
					updates = nil
					continue
				}
				var encoded *C.char
				encoded, updateErr = encode(next)
				if updateErr != nil {
					C.awqt_stop()
					return
				}
				C.awqt_update(encoded)
				C.free(unsafe.Pointer(encoded))
			}
		}
	}()
	status := C.awqt_run()
	close(nativeDone)
	if err := <-done; err != nil {
		return err
	}
	if status == 0 {
		return bridgeError()
	}
	return nil
}

// ListDisplays 与 Run 一样必须从锁定的主 OS 线程调用。
func ListDisplays(ctx context.Context, cfg overlay.Config) ([]overlay.Display, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	value, err := encode(cfg)
	if err != nil {
		return nil, err
	}
	defer C.free(unsafe.Pointer(value))
	result := C.awqt_displays(value)
	if result == nil {
		return nil, bridgeError()
	}
	defer C.awqt_free(result)
	var displays []overlay.Display
	if err := json.Unmarshal([]byte(C.GoString(result)), &displays); err != nil {
		return nil, err
	}
	return displays, ctx.Err()
}
