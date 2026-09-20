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
		return nil, errors.New("overlay: startup timeout must be positive")
	}
	if options.Executable == "" {
		executable, err := os.Executable()
		if err != nil {
			return nil, err
		}
		options.Executable, err = bundledExecutable(executable)
		if err != nil {
			return nil, err
		}
	}
	cfg := DefaultConfig()
	cfg.DisplayID, cfg.Output, cfg.Displays = options.Config.DisplayID, options.Config.Output, options.Config.Displays
	if _, err := cfg.Normalize(); err != nil {
		return nil, err
	}
	input, err := json.Marshal(cfg)
	if err != nil {
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
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("overlay: enumerating displays: %w: %s", err, diagnostics.String())
	}
	var displays []Display
	if err := json.Unmarshal(stdout.Bytes(), &displays); err != nil {
		return nil, fmt.Errorf("overlay: invalid display enumeration: %w", err)
	}
	if len(displays) > 128 {
		return nil, errors.New("overlay: display enumeration exceeds 128 outputs")
	}
	seen := make(map[string]struct{}, len(displays))
	for _, display := range displays {
		if display.ID == "" || len(display.ID) > 512 || strings.ContainsRune(display.ID, '\x00') || !utf8.ValidString(display.ID) || display.Name == "" || len(display.Name) > 4096 || strings.ContainsRune(display.Name, '\x00') || !utf8.ValidString(display.Name) {
			return nil, errors.New("overlay: invalid native display ID or name")
		}
		if _, duplicate := seen[display.ID]; duplicate {
			return nil, errors.New("overlay: duplicate native display ID")
		}
		seen[display.ID] = struct{}{}
	}
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
