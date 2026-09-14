//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package termimage

import (
	"arcana-world/internal/i18n"

	"errors"
	"os"
)

const interactiveSupported = false

func terminalSize(_ *os.File) (dimensions, error) {
	return dimensions{}, errors.New(i18n.T(i18n.TermImageTerminalSizeUnsupported))
}

func pollInput(_ *os.File, _ []byte) (int, error) {
	return 0, errors.New(i18n.T(i18n.TermImagePlatformInputUnsupported))
}
