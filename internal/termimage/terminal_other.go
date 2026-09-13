//go:build !linux && !darwin && !freebsd && !openbsd && !netbsd && !dragonfly

package termimage

import (
	"errors"
	"os"
)

const interactiveSupported = false

func terminalSize(_ *os.File) (dimensions, error) {
	return dimensions{}, errors.New("preview: terminal sizing unsupported on this platform")
}

func pollInput(_ *os.File, _ []byte) (int, error) {
	return 0, errors.New("preview: interactive terminal input unsupported on this platform")
}
