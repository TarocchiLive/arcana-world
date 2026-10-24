//go:build !darwin && !linux && !windows

package rtmpsrt

import "errors"

func openSRT() (func(string) (uintptr, error), error) {
	return nil, errors.New("RTMPSRT native transport supports macOS, Linux and Windows")
}
