package overlay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

// Display 是原生后端返回的稳定标识和用户可见名称。Selected 反映查询时的配置。
type Display struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Selected bool   `json:"selected"`
}

// ListDisplays 通过独立 helper 枚举原生输出，不把 GUI 库链接进 TUI。
// Options.Config 仅使用显示器选择字段；零值表示旧版默认输出。
func ListDisplays(ctx context.Context, options Options) ([]Display, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if options.StartupTimeout == 0 {
		options.StartupTimeout = defaultStartupTimeout
	}
	if options.StartupTimeout < 0 {
		logger.Error("overlay options invalid", "op", "list_displays", "startup_timeout", options.StartupTimeout)
		return nil, errors.New("overlay: startup timeout must be positive")
	}
	if options.Executable == "" {
		executable, err := os.Executable()
		if err != nil {
			logger.Error("overlay helper resolve failed", "op", "list_displays", "backend", options.Config.Backend, "step", "app executable", "err", err)
			return nil, err
		}
		options.Executable, err = bundledExecutable(executable, options.Config.Backend)
		if err != nil {
			logger.Error("overlay helper resolve failed", "op", "list_displays", "backend", options.Config.Backend, "app", executable, "err", err)
			return nil, err
		}
	} else {
		requested := options.Executable
		path, err := canonicalExecutable(options.Executable)
		if err != nil {
			logger.Error("overlay helper resolve failed", "op", "list_displays", "backend", options.Config.Backend, "executable", requested, "err", err)
			return nil, fmt.Errorf("overlay: resolving helper executable: %w", err)
		}
		options.Executable = path
	}
	cfg := DefaultConfig()
	cfg.Backend = options.Config.Backend
	cfg.DisplayID, cfg.Output, cfg.Displays = options.Config.DisplayID, options.Config.Output, options.Config.Displays
	cfg, err := cfg.Normalize()
	if err != nil {
		logger.Error("overlay config invalid", "op", "list_displays", "err", err)
		return nil, err
	}
	input, err := json.Marshal(cfg)
	if err != nil {
		logger.Error("overlay display config encode failed", "err", err)
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, options.StartupTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, options.Executable, "--list-displays", "--display-config-stdin")
	setupChild(cmd)
	cmd.Stdin = bytes.NewReader(input)
	stdout := &displayOutput{}
	diagnostics := &diagnosticBuffer{}
	cmd.Stdout, cmd.Stderr = stdout, diagnostics
	cmd.WaitDelay = defaultShutdownTimeout
	started := time.Now()
	if err := cmd.Run(); err != nil {
		attrs := []any{"backend", cfg.Backend, "executable", options.Executable, "elapsed", time.Since(started), "err", err}
		if cmd.ProcessState != nil {
			attrs = append(attrs, "exit_code", cmd.ProcessState.ExitCode(), "status", cmd.ProcessState.String())
		}
		if text := diagnostics.String(); text != "" {
			attrs = append(attrs, "stderr", text)
		}
		if ctx.Err() != nil {
			attrs = append(attrs, "ctx_err", ctx.Err())
			logger.Error("overlay display enumeration aborted", attrs...)
			return nil, ctx.Err()
		}
		logger.Error("overlay display enumeration failed", attrs...)
		return nil, fmt.Errorf("overlay: enumerating displays: %w: %s", err, diagnostics.String())
	}
	var displays []Display
	if err := json.Unmarshal(stdout.Bytes(), &displays); err != nil {
		logger.Error("overlay display enumeration invalid", "reason", "malformed json", "backend", cfg.Backend, "bytes", len(stdout.Bytes()), "stderr", diagnostics.String(), "err", err)
		return nil, fmt.Errorf("overlay: invalid display enumeration: %w", err)
	}
	if len(displays) > 128 {
		logger.Error("overlay display enumeration invalid", "reason", "too many outputs", "backend", cfg.Backend, "count", len(displays))
		return nil, errors.New("overlay: display enumeration exceeds 128 outputs")
	}
	seen := make(map[string]struct{}, len(displays))
	for _, display := range displays {
		if display.ID == "" || len(display.ID) > 512 || strings.ContainsRune(display.ID, '\x00') || !utf8.ValidString(display.ID) || display.Name == "" || len(display.Name) > 4096 || strings.ContainsRune(display.Name, '\x00') || !utf8.ValidString(display.Name) {
			logger.Error("overlay display enumeration invalid", "reason", "invalid id or name", "backend", cfg.Backend, "id_len", len(display.ID), "name_len", len(display.Name))
			return nil, errors.New("overlay: invalid display ID or name")
		}
		if _, duplicate := seen[display.ID]; duplicate {
			logger.Error("overlay display enumeration invalid", "reason", "duplicate id", "backend", cfg.Backend, "id", display.ID)
			return nil, errors.New("overlay: duplicate display ID")
		}
		seen[display.ID] = struct{}{}
	}
	logger.Debug("overlay displays listed", "backend", cfg.Backend, "count", len(displays), "elapsed", time.Since(started))
	return displays, nil
}

// 原生枚举是小型消息，拒绝失控 helper 的无限标准输出。
type displayOutput struct{ buffer bytes.Buffer }

func (b *displayOutput) Write(p []byte) (int, error) {
	if len(p) > 256*1024-b.buffer.Len() {
		return 0, io.ErrShortBuffer
	}
	return b.buffer.Write(p)
}

func (b *displayOutput) Bytes() []byte { return b.buffer.Bytes() }
