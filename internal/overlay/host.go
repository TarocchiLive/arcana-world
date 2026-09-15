package overlay

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"
)

// Serve 在当前线程运行原生入口；控制连接断开时取消原生生命周期。
func Serve(ctx context.Context, socketPath string, run func(context.Context, Config, <-chan Config, func()) error) error {
	token := os.Getenv(tokenEnvironment)
	_ = os.Unsetenv(tokenEnvironment)
	if token == "" {
		return errors.New("overlay: missing authentication token")
	}
	if run == nil {
		return errors.New("overlay: missing native runner")
	}
	conn, err := (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "unix", socketPath)
	if err != nil {
		return fmt.Errorf("overlay: connecting control socket: %w", err)
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stopClose := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopClose()
	if err = conn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return err
	}
	if err = writeFrame(conn, wireFrame{Type: "hello", Token: token}); err != nil {
		return err
	}
	initial, err := readFrame(conn)
	if err != nil {
		return err
	}
	if initial.Type == "error" {
		return fmt.Errorf("overlay: %s", initial.Error)
	}
	if initial.Type != "config" || initial.Config == nil {
		return errors.New("overlay: expected initial configuration")
	}
	cfg, err := initial.Config.Normalize()
	if err != nil {
		return err
	}
	if err = conn.SetDeadline(time.Time{}); err != nil {
		return err
	}
	updates := make(chan Config, 1)
	ready := make(chan struct{}, 1)
	finished := make(chan error, 1)
	readDone := make(chan error, 1)
	writeDone := make(chan error, 1)
	go func() {
		defer close(updates)
		for {
			frame, readErr := readFrame(conn)
			if readErr != nil {
				if errors.Is(readErr, io.EOF) || ctx.Err() != nil {
					readErr = nil
				}
				readDone <- readErr
				cancel()
				return
			}
			if frame.Type != "config" || frame.Config == nil {
				readDone <- errors.New("overlay: expected configuration update")
				cancel()
				return
			}
			next, normalizeErr := frame.Config.Normalize()
			if normalizeErr != nil {
				readDone <- normalizeErr
				cancel()
				return
			}
			select {
			case updates <- next:
			default:
				select {
				case <-updates:
				default:
				}
				select {
				case updates <- next:
				case <-ctx.Done():
				}
			}
		}
	}()
	go func() {
		var writeErr error
		signal := ready
		for {
			select {
			case <-signal:
				signal = nil
				writeErr = writeFrame(conn, wireFrame{Type: "ready"})
				if writeErr != nil {
					if ctx.Err() != nil {
						writeErr = nil
					}
					cancel()
					writeDone <- writeErr
					return
				}
			case runErr := <-finished:
				if ctx.Err() == nil {
					if runErr != nil {
						writeErr = writeFrame(conn, wireFrame{Type: "error", Error: boundedError(runErr)})
					} else {
						writeErr = writeFrame(conn, wireFrame{Type: "stopped"})
					}
					if ctx.Err() != nil {
						writeErr = nil
					}
				}
				writeDone <- writeErr
				return
			case <-ctx.Done():
				writeDone <- nil
				return
			}
		}
	}()
	var once sync.Once
	runErr := run(ctx, cfg, updates, func() { once.Do(func() { ready <- struct{}{} }) })
	finished <- runErr
	writeErr := <-writeDone
	cancel()
	_ = conn.Close()
	readErr := <-readDone
	if runErr != nil {
		return runErr
	}
	if readErr != nil {
		return readErr
	}
	return writeErr
}

func boundedError(err error) string {
	text := err.Error()
	if len(text) > 8192 {
		return text[:8192]
	}
	return text
}
