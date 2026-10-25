package tts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"arcana-world/internal/helperpath"
	"arcana-world/internal/i18n"
	"arcana-world/internal/tts/meloproto"
)

const (
	maxHelperDiagnosticBytes = 8 << 10
	helperWaitDelay          = 2 * time.Second
)

func resolveHelper(name, explicit string) (string, error) {
	path := explicit
	if path == "" {
		executable, err := os.Executable()
		if err != nil {
			logger.Error("helper resolution failed", "helper", name, "stage", "application_path", "err", err)
			return "", helperFailureError("")
		}
		path, err = helperpath.Installed(executable, name)
		if err != nil {
			logger.Error("helper resolution failed", "helper", name, "stage", "installed_path", "err", err)
			return "", helperFailureError("")
		}
	} else {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			logger.Error("helper resolution failed", "helper", name, "stage", "absolute_path", "err", err)
			return "", helperFailureError("")
		}
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf(i18n.T(i18n.TTSHelperMissing), path)
	}
	if err != nil {
		logger.Error("helper resolution failed", "helper", name, "executable", path, "stage", "stat", "err", err)
		return "", fmt.Errorf(i18n.T(i18n.TTSHelperUnusable), path)
	}
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf(i18n.T(i18n.TTSHelperUnusable), path)
	}
	return path, nil
}

// cappedDiagnostics 消费全部诊断信息，只保留最近的输出：常驻进程会长期运行，
// 最终的失败类别总在最后，不能被早期输出挤掉。
// 仅在 Wait 等待 os/exec 的 stderr 复制协程结束后读取它。
type cappedDiagnostics struct{ data []byte }

func (b *cappedDiagnostics) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) >= maxHelperDiagnosticBytes {
		b.data = append(b.data[:0], p[len(p)-maxHelperDiagnosticBytes:]...)
		return n, nil
	}
	b.data = append(b.data, p...)
	if extra := len(b.data) - maxHelperDiagnosticBytes; extra > 0 {
		b.data = b.data[:copy(b.data, b.data[extra:])]
	}
	return n, nil
}

// helperDiagnosticCategories 把辅助进程 stderr 最后一行中允许进入返回错误的固定类别映射为界面文本。
// 按前缀匹配：类别之后可以携带端点主机等细节，但细节只写入日志，不进入返回的错误。
var helperDiagnosticCategories = []struct {
	prefix string
	key    i18n.Key
}{
	{"audio: empty input", i18n.TTSHelperAudioEmpty},
	{"audio: input produced no PCM", i18n.TTSHelperAudioEmpty},
	{"audio: input exceeds limit", i18n.TTSHelperAudioLimit},
	{"audio: decoded PCM exceeds 120 seconds", i18n.TTSHelperAudioLimit},
	{"audio: unsupported format", i18n.TTSHelperAudioFormat},
	{"audio: invalid MP3 stream", i18n.TTSHelperAudioFormat},
	{"audio: invalid WAV stream", i18n.TTSHelperAudioFormat},
	{"audio: unsupported WAV", i18n.TTSHelperAudioFormat},
	{"audio: invalid FLAC", i18n.TTSHelperAudioFormat},
	{"audio: unsupported FLAC", i18n.TTSHelperAudioFormat},
	{"audio: invalid Ogg Vorbis", i18n.TTSHelperAudioFormat},
	{"audio: unsupported sample rate or channel count", i18n.TTSHelperAudioFormat},
	{"audio: decode PCM", i18n.TTSHelperAudioFormat},
	{"audio: reading input failed", i18n.TTSHelperAudioFormat},
	{"audio: initialize device", i18n.TTSHelperDevice},
	{"audio: device", i18n.TTSHelperDevice},
	{"audio: player", i18n.TTSHelperDevice},
}

// lastDiagnosticLine 返回 stderr 中最后一个非空行；辅助进程会先输出逐次重试的
// 细节行，最终的失败类别总是最后一行。
func lastDiagnosticLine(diagnostic string) string {
	diagnostic = strings.TrimSpace(diagnostic)
	if i := strings.LastIndexByte(diagnostic, '\n'); i >= 0 {
		diagnostic = strings.TrimSpace(diagnostic[i+1:])
	}
	return diagnostic
}

// helperFailureError 把辅助进程的失败转换为返回给用户的错误。
// 不转发任意 stderr 内容：后端可能在其中包含文本或代理凭据。
func helperFailureError(diagnostic string) error {
	last := lastDiagnosticLine(diagnostic)
	for _, category := range helperDiagnosticCategories {
		if strings.HasPrefix(last, category.prefix) {
			return errors.New(i18n.T(category.key))
		}
	}
	return errors.New(i18n.T(i18n.TTSHelperStopped))
}

// residentClient 串行访问常驻进程；关闭取消当前请求，并等待管道与进程全部回收。
type residentClient struct {
	gate   chan struct{}
	mu     sync.Mutex
	closed bool
	cancel context.CancelFunc
	worker *residentProcess
}

type residentProcess struct {
	cmd        *exec.Cmd
	input      io.WriteCloser
	output     io.ReadCloser
	diagnostic *cappedDiagnostics
	waitErr    error
	done       chan struct{}
}

func newResidentClient() *residentClient {
	return &residentClient{gate: make(chan struct{}, 1)}
}

func (c *residentClient) request(ctx context.Context, start func(context.Context) (*exec.Cmd, error), ready string, input []byte, limit int, valid func([]byte) bool) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	select {
	case c.gate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-c.gate }()
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, ErrClosed
	}
	c.cancel = cancel
	c.mu.Unlock()
	defer func() { c.mu.Lock(); c.cancel = nil; c.mu.Unlock() }()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c.worker != nil {
		select {
		case <-c.worker.done:
			c.stopWorker()
		default:
		}
	}
	if c.worker == nil {
		cmd, err := start(ctx)
		if err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		p := &residentProcess{cmd: cmd, diagnostic: new(cappedDiagnostics), done: make(chan struct{})}
		cmd.WaitDelay = helperWaitDelay
		cmd.Stderr = p.diagnostic
		p.input, err = cmd.StdinPipe()
		if err != nil {
			logger.Error("resident worker setup failed", "executable", cmd.Path, "stage", "stdin_pipe", "err", err)
			return nil, helperFailureError("")
		}
		// stdout 自主管理，避免后台 Wait 提前关闭尚未读取的回复帧。
		output, writer, err := os.Pipe()
		if err != nil {
			logger.Error("resident worker setup failed", "executable", cmd.Path, "stage", "stdout_pipe", "err", err)
			_ = p.input.Close()
			return nil, helperFailureError("")
		}
		p.output = output
		cmd.Stdout = writer
		if err = cmd.Start(); err != nil {
			_ = p.input.Close()
			_ = output.Close()
			_ = writer.Close()
			logger.Error("resident worker start failed", "executable", cmd.Path, "err", err)
			return nil, helperFailureError("")
		}
		_ = writer.Close()
		go func() { p.waitErr = cmd.Wait(); close(p.done) }()
		c.worker = p
		logger.Debug("resident worker started", "pid", cmd.Process.Pid, "executable", cmd.Path)
		if _, err = c.exchange(ctx, nil, len(ready), func(data []byte) bool { return string(data) == ready }); err != nil {
			return nil, err
		}
	}
	return c.exchange(ctx, input, limit, valid)
}

func (c *residentClient) exchange(ctx context.Context, input []byte, limit int, valid func([]byte) bool) ([]byte, error) {
	p := c.worker
	started := time.Now()
	stage := "request"
	if input == nil {
		stage = "ready"
	}
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	go func() {
		if input != nil {
			if err := meloproto.Write(p.input, input); err != nil {
				done <- result{err: err}
				return
			}
		}
		data, err := meloproto.Read(p.output, limit)
		if err == nil && valid != nil && !valid(data) {
			err = errors.New("invalid worker response")
		}
		done <- result{data, err}
	}()
	var r result
	select {
	case r = <-done:
	case <-ctx.Done():
		c.stopWorker()
		<-done
		// 跳过与停止播报属于正常取消；超时等其他原因才值得告警。
		level := slog.LevelWarn
		if errors.Is(ctx.Err(), context.Canceled) {
			level = slog.LevelDebug
		}
		logger.Log(context.Background(), level, "resident worker interrupted", "pid", p.cmd.Process.Pid, "executable", p.cmd.Path, "stage", stage, "elapsed", time.Since(started), "err", ctx.Err(), "process_error", p.waitErr)
		return nil, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		c.stopWorker()
		return nil, err
	}
	if r.err != nil {
		c.stopWorker()
		diagnostic := strings.TrimSpace(string(p.diagnostic.data))
		// 只去掉 worker 原样回显的整段文本；音频输入远长于受限的 stderr，不做无用复制。
		if len(input) != 0 && len(input) <= len(diagnostic) {
			diagnostic = strings.ReplaceAll(diagnostic, string(input), "[REDACTED]")
		}
		// 细节只写入日志；stderr 可能含文本片段或代理凭据，返回的错误仅含固定类别。
		logger.Error("resident worker failed", "pid", p.cmd.Process.Pid, "executable", p.cmd.Path, "stage", stage, "elapsed", time.Since(started), "stderr", diagnostic, "err", r.err, "process_error", p.waitErr)
		return nil, helperFailureError(string(p.diagnostic.data))
	}
	return r.data, nil
}

func (c *residentClient) stopWorker() {
	if c.worker == nil {
		return
	}
	p := c.worker
	c.worker = nil
	_ = p.input.Close()
	_ = p.cmd.Process.Kill()
	_ = p.output.Close()
	<-p.done
	logger.Debug("resident worker reaped", "pid", p.cmd.Process.Pid, "executable", p.cmd.Path, "process_error", p.waitErr)
}

func (c *residentClient) Close() error {
	c.mu.Lock()
	c.closed = true
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
	c.gate <- struct{}{}
	defer func() { <-c.gate }()
	if c.worker != nil {
		// 空闲关闭送 EOF，保留原生析构机会；异常进程最多等待两秒。
		_ = c.worker.input.Close()
		timer := time.NewTimer(helperWaitDelay)
		select {
		case <-c.worker.done:
		case <-timer.C:
		}
		timer.Stop()
	}
	c.stopWorker()
	return nil
}
