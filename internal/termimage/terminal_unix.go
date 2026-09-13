//go:build linux || darwin || freebsd || openbsd || netbsd || dragonfly

package termimage

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

const interactiveSupported = true

func terminalSize(f *os.File) (dimensions, error) {
	size, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		return dimensions{}, err
	}
	return dimensions{cols: int(size.Col), rows: int(size.Row), width: int(size.Xpixel), height: int(size.Ypixel)}, nil
}

// Bounded polling avoids reader goroutines, descriptor flag mutation and signal
// handlers. The only reader is Run, after Bubble Tea has released the terminal.
func pollInput(f *os.File, data []byte) (int, error) {
	fds := []unix.PollFd{{Fd: int32(f.Fd()), Events: unix.POLLIN}}
	_, err := unix.Poll(fds, 100)
	if errors.Is(err, unix.EINTR) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if fds[0].Revents&unix.POLLIN != 0 {
		n, err := unix.Read(int(f.Fd()), data)
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			return 0, nil
		}
		if n == 0 && err == nil {
			return 0, io.EOF
		}
		return max(0, n), err
	}
	if fds[0].Revents&(unix.POLLHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
		return 0, io.EOF
	}
	return 0, nil
}
