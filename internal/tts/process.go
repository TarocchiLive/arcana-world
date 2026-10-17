package tts

import (
	"bytes"
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
	"time"

	"arcana-world/internal/helperpath"
)

const (
	maxHelperDiagnosticBytes = 8 << 10
	maxHelperLogStderrBytes  = 4 << 10
	helperWaitDelay          = 2 * time.Second
)

func resolveHelper(name, explicit string) (string, error) {
	path := explicit
	if path == "" {
		executable, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("tts: locating application: %w", err)
		}
		path, err = helperpath.Installed(executable, name)
		if err != nil {
			return "", fmt.Errorf("tts: resolving application: %w", err)
		}
	} else {
		var err error
		path, err = filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("tts: resolving helper: %w", err)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("tts: helper %q unavailable; build the TTS helper or extract the complete portable package: %w", path, err)
	}
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("tts: helper %q is not executable; rebuild the TTS helper or extract the complete portable package", path)
	}
	return path, nil
}

// cappedDiagnostics 消费全部诊断信息，但不会保留无界输出。
// 仅在 Wait 等待 os/exec 的 stderr 复制协程结束后读取它。
type cappedDiagnostics struct{ data []byte }

func (b *cappedDiagnostics) Write(p []byte) (int, error) {
	n := len(p)
	if remaining := maxHelperDiagnosticBytes - len(b.data); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		b.data = append(b.data, p...)
	}
	return n, nil
}

// helperDiagnosticCategories 是辅助进程 stderr 最后一行中允许进入返回错误的固定类别。
// 按前缀匹配：类别之后可以携带端点主机等细节，但细节只写入日志，不进入返回的错误。
var helperDiagnosticCategories = []string{
	"tts: synthesis network timeout", "tts: synthesis handshake rejected",
	"tts: synthesis network or protocol failure", "tts: invalid synthesis input",
	"tts: synthesis returned empty audio", "tts: synthesis audio exceeds limit",
	"audio: empty MP3", "audio: MP3 exceeds 8 MiB", "audio: truncated ID3 header",
	"audio: invalid ID3 length", "audio: ID3 metadata exceeds source bounds or 1 MiB",
	"audio: invalid MP3 stream", "audio: MP3 sample rate must be 24000 Hz",
	"audio: decoded PCM exceeds 120 seconds", "audio: reading MP3 input failed",
	"audio: MP3 produced no PCM", "audio: nil context",
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
// 返回的错误不带 "tts:" 前缀，由上层（Manager）统一添加一次。
func helperFailureError(diagnostic string, waitErr error) error {
	last := lastDiagnosticLine(diagnostic)
	for _, category := range helperDiagnosticCategories {
		if strings.HasPrefix(last, category) {
			return fmt.Errorf("%s: %w", strings.TrimPrefix(category, "tts: "), waitErr)
		}
	}
	for _, category := range []string{"audio: initialize device:", "audio: device:", "audio: player:", "audio: decode PCM:"} {
		if strings.HasPrefix(last, category) {
			return fmt.Errorf("%s helper failed: %w", category, waitErr)
		}
	}
	return fmt.Errorf("helper failed: %w", waitErr)
}

// runHelper 运行一次辅助进程。args 与 executable 会写入日志，因此调用方绝不能
// 把待朗读文本放进参数；文本只通过 stdin 传递。
func runHelper(ctx context.Context, executable string, args []string, input []byte, maxOutput int, env []string) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxOutput < 0 {
		return nil, errors.New("invalid output limit")
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.WaitDelay = helperWaitDelay
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(input)
	diagnostics := new(cappedDiagnostics)
	cmd.Stderr = diagnostics
	argText := strings.Join(args, " ")
	started := time.Now()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		logger.Error("helper pipe failed", "executable", executable, "args", argText, "err", err)
		return nil, fmt.Errorf("helper output pipe: %w", err)
	}
	if err = cmd.Start(); err != nil {
		stdout.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		logger.Error("helper start failed", "executable", executable, "args", argText, "elapsed", time.Since(started), "err", err)
		return nil, fmt.Errorf("starting helper: %w", err)
	}
	logger.Debug("helper started", "executable", executable, "args", argText, "pid", cmd.Process.Pid, "input_bytes", len(input))
	output, readErr := io.ReadAll(io.LimitReader(stdout, int64(maxOutput)+1))
	oversized := len(output) > maxOutput
	if readErr != nil || oversized {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	elapsed := time.Since(started)
	exitCode := cmd.ProcessState.ExitCode()
	pid := cmd.Process.Pid
	if err := ctx.Err(); err != nil {
		// 超时是需要关注的降级；主动取消属于正常流程。
		level := slog.LevelDebug
		if errors.Is(err, context.DeadlineExceeded) {
			level = slog.LevelWarn
		}
		logger.Log(ctx, level, "helper aborted", "executable", executable, "args", argText, "pid", pid, "exit_code", exitCode, "elapsed", elapsed, "err", err)
		return nil, err
	}
	if oversized {
		logger.Error("helper output exceeds limit", "executable", executable, "args", argText, "pid", pid, "elapsed", elapsed, "limit", maxOutput)
		return nil, errors.New("helper output exceeds limit")
	}
	if readErr != nil {
		logger.Error("helper output read failed", "executable", executable, "args", argText, "pid", pid, "elapsed", elapsed, "err", readErr)
		return nil, fmt.Errorf("reading helper output: %w", readErr)
	}
	if waitErr != nil {
		stderr := strings.TrimSpace(string(diagnostics.data))
		if len(stderr) > maxHelperLogStderrBytes {
			stderr = stderr[:maxHelperLogStderrBytes] + "…"
		}
		logger.Error("helper failed", "executable", executable, "args", argText, "pid", pid, "exit_code", exitCode, "elapsed", elapsed, "stderr", stderr, "err", waitErr)
		return nil, helperFailureError(string(diagnostics.data), waitErr)
	}
	logger.Debug("helper exited", "executable", executable, "pid", pid, "exit_code", exitCode, "elapsed", elapsed, "output_bytes", len(output))
	return output, nil
}
