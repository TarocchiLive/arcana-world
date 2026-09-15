//go:build (!darwin && !windows && !linux) || (darwin && !cgo) || (linux && (!cgo || !wayland))

package native

import (
	"context"
	"errors"
	"runtime"

	"arcana-world/internal/overlay"
)

func runPlatform(context.Context, overlay.Config, <-chan overlay.Config, func()) error {
	switch runtime.GOOS {
	case "darwin":
		return errors.New("overlay: macOS requires an AppKit build with CGO_ENABLED=1; run make overlay on macOS")
	case "linux":
		return errors.New("overlay: Linux requires a native Wayland build with CGO_ENABLED=1 and -tags wayland (make overlay), plus wayland-client and pangocairo development packages; X11 is not implemented")
	default:
		return errors.New("overlay: this operating system is not supported")
	}
}
