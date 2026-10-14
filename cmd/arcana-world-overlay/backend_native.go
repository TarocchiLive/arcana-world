//go:build !qt

package main

import (
	"errors"
	"flag"

	"arcana-world/internal/overlay"
	"arcana-world/internal/overlay/native"
)

var runBackend = native.Run
var listBackendDisplays = native.ListDisplays

const backendID = overlay.BackendNative

const backendHelp = "Native click-through overlay: macOS, Windows 10 1803+, KDE/niri Wayland. Linux requires layer-shell; no ordinary-window or X11 fallback."

func registerBackendFlags(_ *flag.FlagSet, _ *overlay.Config) {}

func configureBackend(reducedMotion, unlocked bool) error {
	if reducedMotion || unlocked {
		return errors.New("-reduced-motion and -unlocked require the Elegant overlay helper")
	}
	return nil
}
