package overlay

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"arcana-world/internal/helperpath"
)

const (
	defaultStartupTimeout  = 10 * time.Second
	defaultShutdownTimeout = 2 * time.Second
	defaultUpdateInterval  = time.Second / 60
	hostAuthTimeout        = time.Second
	// Leave room within the Unix socket path limits of all supported platforms.
	socketPathLimit = 100
)

// Options 控制独立原生子进程及其有界通信生命周期。
type Options struct {
	Executable      string
	Config          Config
	StartupTimeout  time.Duration
	ShutdownTimeout time.Duration
	UpdateInterval  time.Duration
	RuntimeDir      string
}

// Manager 保存最新完整快照；设置成功不表示原生窗口已经绘制。
type Manager struct {
	mu     sync.Mutex
	cfg    Config
	err    error
	closed bool
	done   chan struct{}
	dirty  chan struct{}
	cancel context.CancelFunc
}

// Resolve the real installation directory, not the working directory or a launcher symlink.
func bundledExecutable(executable string) (string, error) {
	path, err := helperpath.Installed(executable, "arcana-world-overlay")
	if err != nil {
		return "", fmt.Errorf("overlay: resolving application path: %w", err)
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("overlay: bundled executable %q is missing; extract the complete portable package again: %w", path, err)
	}
	if err != nil {
		return "", fmt.Errorf("overlay: accessing bundled executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("overlay: bundled executable %q is not a regular file; extract the complete portable package again", path)
	}
	return path, nil
}

// Start 仅在认证完成且原生窗口首次绘制后返回。
func Start(ctx context.Context, options Options) (*Manager, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cfg, err := options.Config.Normalize()
	if err != nil {
		return nil, err
	}
	if options.StartupTimeout == 0 {
		options.StartupTimeout = defaultStartupTimeout
	}
	if options.ShutdownTimeout == 0 {
		options.ShutdownTimeout = defaultShutdownTimeout
	}
	if options.UpdateInterval == 0 {
		options.UpdateInterval = defaultUpdateInterval
	}
	if options.StartupTimeout < 0 || options.ShutdownTimeout < 0 || options.UpdateInterval < 0 {
		return nil, errors.New("overlay: timeouts and update interval must be positive")
	}
	if options.Executable == "" {
		executable, e := os.Executable()
		if e != nil {
			return nil, e
		}
		options.Executable, e = bundledExecutable(executable)
		if e != nil {
			return nil, e
		}
	}
	directory, err := privateSocketDir(options.RuntimeDir)
	if err != nil {
		return nil, err
	}
	var listener *net.UnixListener
	transferred := false
	defer func() {
		if transferred {
			return
		}
		if listener != nil {
			_ = listener.Close()
		}
		_ = os.RemoveAll(directory)
	}()
	socketPath := filepath.Join(directory, "s")
	if len(socketPath) >= socketPathLimit {
		return nil, fmt.Errorf("overlay: runtime directory produces a Unix socket path of %d bytes (must be below %d)", len(socketPath), socketPathLimit)
	}
	listener, err = net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		return nil, fmt.Errorf("overlay: Unix sockets require a supported OS (Windows 10 1803 or newer): %w", err)
	}
	if err = secureSocket(socketPath); err != nil {
		return nil, err
	}
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(secret[:])
	cmd := exec.Command(options.Executable, "-socket", socketPath)
	setupChild(cmd)
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(key, tokenEnvironment) {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Env = append(cmd.Env, tokenEnvironment+"="+token)
	diagnostics := &diagnosticBuffer{}
	cmd.Stderr = diagnostics
	cmd.WaitDelay = options.ShutdownTimeout
	if err = cmd.Start(); err != nil {
		return nil, fmt.Errorf("overlay: starting child: %w", err)
	}
	childDone := make(chan struct{})
	var childErr error
	go func() { childErr = cmd.Wait(); close(childDone) }()
	managedCtx, cancel := context.WithCancel(ctx)
	manager := &Manager{cfg: cfg, done: make(chan struct{}), dirty: make(chan struct{}, 1), cancel: cancel}
	ready := make(chan struct{})
	go manager.manage(managedCtx, options, listener, directory, cmd, childDone, &childErr, diagnostics, token, ready)
	transferred = true // manage now owns the listener, directory, and child.
	select {
	case <-ready:
		if err := ctx.Err(); err != nil {
			_ = manager.Close()
			return nil, err
		}
		return manager, nil
	case <-manager.done:
		err = manager.Err()
		if err == nil {
			err = ctx.Err()
			if err == nil {
				err = errors.New("overlay: child stopped during startup")
			}
		}
		return nil, err
	}
}

func (m *Manager) manage(ctx context.Context, options Options, listener *net.UnixListener, directory string, cmd *exec.Cmd, childDone <-chan struct{}, childErr *error, diagnostics *diagnosticBuffer, token string, ready chan<- struct{}) {
	var conn net.Conn
	var workers sync.WaitGroup
	var failure error
	becameReady := false
	startup, finishStartup := context.WithTimeout(ctx, options.StartupTimeout)
	defer finishStartup()
	stopListener := context.AfterFunc(startup, func() { _ = listener.Close() })
	defer stopListener()
	defer func() {
		// 先读取外部取消原因，再取消内部协程；不能把所有退出都改写为 canceled。
		if ctx.Err() != nil {
			if becameReady {
				failure = nil
			} else {
				failure = ctx.Err()
			}
		} else if !becameReady && startup.Err() != nil {
			failure = startup.Err()
		}
		m.mu.Lock()
		m.closed = true
		m.mu.Unlock()
		m.cancel()
		_ = listener.Close()
		if conn != nil {
			_ = conn.Close()
		}
		shutdownDeadline := time.Now().Add(options.ShutdownTimeout)
		timer := time.NewTimer(options.ShutdownTimeout / 2)
		select {
		case <-childDone:
			timer.Stop()
		case <-timer.C:
			_ = cmd.Process.Kill()
			reapTimer := time.NewTimer(time.Until(shutdownDeadline))
			select {
			case <-childDone:
				reapTimer.Stop()
			case <-reapTimer.C:
				if failure == nil {
					failure = errors.New("overlay: child did not exit after termination")
				}
			}
		}
		workers.Wait()
		_ = os.RemoveAll(directory)
		if failure != nil {
			if text := diagnostics.String(); text != "" {
				failure = fmt.Errorf("%w; child stderr: %s", failure, text)
			}
		}
		m.mu.Lock()
		m.err = failure
		m.mu.Unlock()
		close(m.done)
	}()
	accepted := make(chan net.Conn)
	acceptErr := make(chan error, 1)
	workers.Add(1)
	go func() {
		defer workers.Done()
		c, err := acceptHost(startup, listener, token)
		if err != nil {
			acceptErr <- err
			return
		}
		select {
		case accepted <- c:
		case <-startup.Done():
			_ = c.Close()
		}
	}()
	select {
	case conn = <-accepted:
	case failure = <-acceptErr:
		return
	case <-childDone:
		failure = fmt.Errorf("overlay: child exited before connecting: %v", *childErr)
		return
	case <-startup.Done():
		failure = startup.Err()
		return
	}
	_ = listener.Close()
	stopConn := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stopConn()
	// 写帧自己的超时不能延长整个启动预算；到期直接中断正在进行的 I/O。
	stopStartupConn := context.AfterFunc(startup, func() { _ = conn.Close() })
	defer stopStartupConn()
	deadline, _ := startup.Deadline()
	_ = conn.SetDeadline(deadline)
	initial := m.Snapshot()
	if err := writeFrame(conn, wireFrame{Type: "config", Config: &initial}); err != nil {
		failure = err
		return
	}
	frame, err := readFrame(conn)
	if err != nil {
		failure = fmt.Errorf("overlay: waiting for native readiness: %w", err)
		return
	}
	if frame.Type != "ready" {
		failure = fmt.Errorf("overlay: native startup failed (%s): %s", frame.Type, frame.Error)
		return
	}
	if !stopStartupConn() || startup.Err() != nil {
		failure = startup.Err()
		return
	}
	finishStartup()
	_ = conn.SetDeadline(time.Time{})
	becameReady = true
	close(ready)
	failures := make(chan error, 2)
	workers.Add(2)
	go func() { defer workers.Done(); failures <- m.writeUpdates(ctx, conn, options.UpdateInterval, initial) }()
	go func() {
		defer workers.Done()
		next, err := readFrame(conn)
		if err == nil {
			switch next.Type {
			case "error":
				err = fmt.Errorf("overlay: native error: %s", next.Error)
			case "stopped":
				// 原生窗口主动正常退出与协议错误不同，不触发自动重启。
			default:
				err = fmt.Errorf("overlay: unexpected host message %q", next.Type)
			}
		}
		failures <- err
	}()
	select {
	case failure = <-failures:
	case <-childDone:
		if *childErr != nil {
			failure = fmt.Errorf("overlay: child exited unexpectedly: %w", *childErr)
		}
	case <-ctx.Done():
	}
}

// 未认证连接不能占用唯一宿主名额。每次认证最多等待一秒，且受总启动预算约束。
func acceptHost(ctx context.Context, listener *net.UnixListener, token string) (net.Conn, error) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return nil, err
		}
		stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
		deadline := time.Now().Add(hostAuthTimeout)
		if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
			deadline = limit
		}
		_ = conn.SetDeadline(deadline)
		hello, readErr := readFrame(conn)
		authenticated := subtle.ConstantTimeCompare([]byte(hello.Token), []byte(token)) == 1
		active := stop()
		if !active || ctx.Err() != nil {
			_ = conn.Close()
			return nil, ctx.Err()
		}
		if !authenticated {
			_ = conn.Close()
			continue
		}
		if readErr != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("overlay: authentication handshake: %w", readErr)
		}
		if hello.Type != "hello" {
			_ = conn.Close()
			return nil, errors.New("overlay: expected authenticated hello")
		}
		return conn, nil
	}
}

func (m *Manager) writeUpdates(ctx context.Context, conn net.Conn, interval time.Duration, last Config) error {
	nextWrite := time.Now().Add(interval)
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-m.dirty:
		}
		if delay := time.Until(nextWrite); delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil
			}
		}
		cfg := m.Snapshot()
		if cfg == last {
			continue
		}
		if err := writeFrame(conn, wireFrame{Type: "config", Config: &cfg}); err != nil {
			return err
		}
		last = cfg
		nextWrite = time.Now().Add(interval)
	}
}

func (m *Manager) Done() <-chan struct{} { return m.done }
func (m *Manager) Err() error            { m.mu.Lock(); defer m.mu.Unlock(); return m.err }
func (m *Manager) Close() error {
	m.mu.Lock()
	m.closed = true
	m.mu.Unlock()
	m.cancel()
	<-m.done
	return m.Err()
}

type diagnosticBuffer struct {
	mu   sync.Mutex
	data []byte
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	const limit = 16 * 1024
	if n >= limit {
		b.data = append(b.data[:0], p[n-limit:]...)
	} else {
		if len(b.data)+n > limit {
			copy(b.data, b.data[len(b.data)+n-limit:])
			b.data = b.data[:limit-n]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}
func (b *diagnosticBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return strings.TrimSpace(string(b.data))
}
