package tts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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

// cappedDiagnostics consumes all diagnostics without retaining unbounded output.
// It is only read after Wait joins os/exec's stderr copying goroutine.
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

func runHelper(ctx context.Context, executable string, args []string, input []byte, maxOutput int, env []string) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("tts: nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if maxOutput < 0 {
		return nil, errors.New("tts: invalid output limit")
	}
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.WaitDelay = helperWaitDelay
	cmd.Env = env
	cmd.Stdin = bytes.NewReader(input)
	diagnostics := new(cappedDiagnostics)
	cmd.Stderr = diagnostics
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("tts: helper output pipe: %w", err)
	}
	if err = cmd.Start(); err != nil {
		stdout.Close()
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("tts: starting helper: %w", err)
	}
	output, readErr := io.ReadAll(io.LimitReader(stdout, int64(maxOutput)+1))
	oversized := len(output) > maxOutput
	if readErr != nil || oversized {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if oversized {
		return nil, errors.New("tts: helper output exceeds limit")
	}
	if readErr != nil {
		return nil, fmt.Errorf("tts: reading helper output: %w", readErr)
	}
	// Do not forward arbitrary stderr: a backend can include text or proxy credentials.
	if waitErr != nil {
		diagnostic := strings.TrimSpace(string(diagnostics.data))
		switch diagnostic {
		case "tts: synthesis network timeout", "tts: synthesis handshake rejected",
			"tts: synthesis network or protocol failure", "tts: invalid synthesis input",
			"tts: synthesis returned empty audio", "tts: synthesis audio exceeds limit",
			"audio: empty MP3", "audio: MP3 exceeds 8 MiB", "audio: truncated ID3 header",
			"audio: invalid ID3 length", "audio: ID3 metadata exceeds source bounds or 1 MiB",
			"audio: invalid MP3 stream", "audio: MP3 sample rate must be 24000 Hz",
			"audio: decoded PCM exceeds 120 seconds", "audio: reading MP3 input failed",
			"audio: MP3 produced no PCM", "audio: nil context":
			return nil, fmt.Errorf("%s: %w", diagnostic, waitErr)
		}
		for _, category := range []string{"audio: initialize device:", "audio: device:", "audio: player:", "audio: decode PCM:"} {
			if strings.HasPrefix(diagnostic, category) {
				return nil, fmt.Errorf("%s helper failed: %w", category, waitErr)
			}
		}
		return nil, fmt.Errorf("tts: helper failed: %w", waitErr)
	}
	return output, nil
}
