//go:build !windows

package bot

import (
	"errors"
	"io"
	"os"
	"sync"

	"golang.org/x/sys/unix"
)

// Stdio 为独占的机器人进程创建可由 Close 中断的标准流。
// 关闭副本后恢复共享描述符标志，避免影响启动它的终端或父进程。
func Stdio() (io.ReadCloser, io.WriteCloser, error) {
	input, err := pollableStdio(os.Stdin)
	if err != nil {
		return nil, nil, err
	}
	output, err := pollableStdio(os.Stdout)
	if err != nil {
		_ = input.Close()
		return nil, nil, err
	}
	return input, output, nil
}

func pollableStdio(original *os.File) (*stdioStream, error) {
	originalFD := int(original.Fd())
	flags, err := unix.FcntlInt(uintptr(originalFD), unix.F_GETFL, 0)
	if err != nil {
		return nil, errors.New("bot stdio initialization failed")
	}
	fd, err := unix.Dup(originalFD)
	if err != nil {
		return nil, errors.New("bot stdio initialization failed")
	}
	unix.CloseOnExec(fd)
	// 必须先设置非阻塞再构造 os.File，让 Go 为管道注册运行时轮询器。
	if err := unix.SetNonblock(fd, true); err != nil {
		_ = unix.Close(fd)
		return nil, errors.New("bot stdio initialization failed")
	}
	file := os.NewFile(uintptr(fd), original.Name())
	if file == nil {
		_ = unix.Close(fd)
		_, _ = unix.FcntlInt(uintptr(originalFD), unix.F_SETFL, flags)
		return nil, errors.New("bot stdio initialization failed")
	}
	return &stdioStream{File: file, originalFD: originalFD, flags: flags}, nil
}

type stdioStream struct {
	*os.File
	originalFD int
	flags      int
	once       sync.Once
	closeErr   error
}

func (s *stdioStream) Close() error {
	s.once.Do(func() {
		// 先中断轮询中的 I/O，再恢复阻塞模式，避免把新 I/O 留在阻塞系统调用中。
		s.closeErr = s.File.Close()
		if _, err := unix.FcntlInt(uintptr(s.originalFD), unix.F_SETFL, s.flags); err != nil {
			s.closeErr = errors.New("bot stdio cleanup failed")
		}
	})
	return s.closeErr
}
