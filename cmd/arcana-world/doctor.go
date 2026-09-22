package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"arcana-world/internal/bili"
	"arcana-world/internal/helperpath"
	"arcana-world/internal/store"
)

type doctorReport struct {
	out      io.Writer
	failures int
	writeErr error
}

func (r *doctorReport) line(level, topic, message string, args ...any) {
	if level == "ERROR" {
		r.failures++
	}
	if r.writeErr == nil {
		_, r.writeErr = fmt.Fprintf(r.out, "[%s] %s: %s\n", level, topic, fmt.Sprintf(message, args...))
	}
}

func (r *doctorReport) issue(required bool, topic, message string, args ...any) {
	level := "WARN"
	if required {
		level = "ERROR"
	}
	r.line(level, topic, message, args...)
}

func runDoctor(ctx context.Context, out io.Writer, dir string, options store.Options, overlayExecutable string, overlayOverride, ttsOverride *bool) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r := &doctorReport{out: out}
	r.line("OK", "scope", "read-only diagnostics; helpers, audio devices, OBS and Bilibili are not started or contacted")
	resolved, err := store.ResolveDir(dir)
	if err != nil {
		r.line("ERROR", "config", "cannot resolve configuration directory")
	} else {
		r.line("OK", "config path", "%q", filepath.Join(resolved, "config.json"))
		cfg, readErr := store.ReadConfig(resolved)
		if readErr != nil {
			// 加载器错误可能包含配置值；绝不打印这些错误。
			r.line("ERROR", "config", "cannot read configuration: invalid contents, file type or access permissions")
		} else {
			cfg = options.Overrides.Apply(cfg)
			client, proxyErr := bili.New(cfg.Proxy)
			if proxyErr != nil {
				r.line("ERROR", "proxy", "invalid proxy configuration; value omitted")
			} else {
				client.HTTP.CloseIdleConnections()
			}
			if _, statErr := os.Lstat(filepath.Join(resolved, "config.json")); errors.Is(statErr, os.ErrNotExist) {
				r.line("OK", "config", "no saved configuration; defaults apply without creating files")
			} else {
				r.line("OK", "config", "saved configuration loaded; runtime overrides applied in memory only")
			}
			proxyMode := "configured URL (redacted)"
			if cfg.Proxy == "" {
				proxyMode = "environment"
			}
			if cfg.Proxy == "direct" {
				proxyMode = "direct"
			}
			r.line("OK", "runtime", "proxy=%s; OBS auto-connect=%t; OBS auto-stream=%t", proxyMode, cfg.OBSAutoConnect, cfg.OBSAutoStream)
			overlayEnabled := cfg.Overlay.Enabled
			if overlayOverride != nil {
				overlayEnabled = *overlayOverride
			}
			ttsEnabled := ttsOverride != nil && *ttsOverride
			r.line("OK", "features", "overlay=%t; TTS=%t; only explicit enables make missing optional prerequisites blocking", overlayEnabled, ttsEnabled)
		}
		r.credentials(ctx, resolved, options.CredentialBackend)
	}
	// 可选辅助程序不会阻止终端 UI 运行。只有显式启用时，
	// 已知缺失的前置条件才会成为阻断问题。
	overlayRequired := overlayOverride != nil && *overlayOverride
	ttsRequired := ttsOverride != nil && *ttsOverride
	r.helper("overlay", "arcana-world-overlay", overlayExecutable, overlayRequired)
	r.helper("tts", "arcana-world-tts", "", ttsRequired)
	r.graphics(overlayRequired)
	r.audio(ttsRequired)
	if ctx.Err() != nil {
		r.line("ERROR", "deadline", "diagnostics canceled or exceeded the five-second deadline")
	}
	r.line("WARN", "limits", "no credentials read, wallet unlocked, service activated, helper executed or device opened; permissions at execution time and network access remain unverified")
	if r.writeErr != nil {
		return r.writeErr
	}
	if r.failures != 0 {
		return fmt.Errorf("doctor: %d blocking issue(s)", r.failures)
	}
	return nil
}

func (r *doctorReport) credentials(ctx context.Context, dir, mode string) {
	if mode == "" {
		mode = "auto"
	}
	switch mode {
	case "memory":
		r.line("OK", "credentials", "memory selected; credentials last only for this process")
		return
	case "auto", "file", "system":
	default:
		r.line("ERROR", "credentials", "invalid backend selection")
		return
	}
	if mode != "system" {
		if info, err := os.Lstat(filepath.Join(dir, "credentials")); err == nil && !info.IsDir() {
			r.line("ERROR", "credentials", "credential directory is not a regular directory")
			return
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			r.line("ERROR", "credentials", "cannot inspect credential directory metadata")
			return
		}
		path := filepath.Join(dir, "credentials", "secrets.json")
		info, err := os.Lstat(path)
		if err == nil {
			if !info.Mode().IsRegular() {
				r.line("ERROR", "credentials", "existing credential file is not a regular file; contents not read")
				return
			}
			r.line("OK", "credentials", "file selected%s; existing file contents not read", doctorAutoReason(mode))
			r.line("WARN", "credentials", "file access, secure permissions and write access not tested; no permission changes made")
			return
		}
		if !errors.Is(err, os.ErrNotExist) {
			r.line("ERROR", "credentials", "cannot inspect credential file metadata")
			return
		}
		if mode == "file" {
			r.line("OK", "credentials", "file selected; no existing credential file; write access not tested")
			return
		}
	}
	switch runtime.GOOS {
	case "darwin":
		r.line("WARN", "credentials", "system backend: macOS Keychain provided; access and unlock state not tested")
	case "windows":
		r.line("WARN", "credentials", "system backend: Windows Credential Manager provided; access not tested")
	case "linux":
		available, known, detail := doctorSecretService(ctx)
		if !known {
			r.line("WARN", "credentials", "%s; backend selection remains unverified", detail)
		} else if available {
			r.line("WARN", "credentials", "system backend expected: %s; access and unlock state not tested", detail)
		} else if mode == "auto" {
			r.line("WARN", "credentials", "file fallback expected: %s; write access not tested", detail)
		} else {
			r.line("ERROR", "credentials", "system backend unavailable: %s", detail)
		}
	default:
		r.issue(mode == "system", "credentials", "native credential store unsupported on this platform")
	}
}

func doctorAutoReason(mode string) string {
	if mode == "auto" {
		return " by auto because a credential file already exists"
	}
	return " explicitly"
}

func (r *doctorReport) helper(topic, name, explicit string, required bool) {
	path := explicit
	var err error
	if path == "" {
		var executable string
		executable, err = os.Executable()
		if err == nil {
			path, err = helperpath.Installed(executable, name)
		}
	} else {
		path, err = filepath.Abs(path)
	}
	if err != nil {
		r.issue(required, topic, "cannot resolve helper path")
		return
	}
	r.line("OK", topic+" path", "%q", path)
	info, err := os.Stat(path)
	if err != nil {
		r.issue(required, topic, "helper missing or inaccessible; extract the complete portable package or build the helper")
		return
	}
	if !info.Mode().IsRegular() || runtime.GOOS != "windows" && info.Mode().Perm()&0111 == 0 {
		r.issue(required, topic, "helper is not a regular executable file")
		return
	}
	r.inspectBinary(topic, path, required)
}

func (r *doctorReport) graphics(required bool) {
	switch runtime.GOOS {
	case "linux":
		display := os.Getenv("WAYLAND_DISPLAY")
		if display == "" {
			r.issue(required, "graphics", "WAYLAND_DISPLAY is absent; native overlay requires Wayland, not X11")
			return
		}
		if !filepath.IsAbs(display) {
			root := os.Getenv("XDG_RUNTIME_DIR")
			if !filepath.IsAbs(root) {
				r.issue(required, "graphics", "Wayland display is relative but XDG_RUNTIME_DIR is absent or not absolute")
				return
			}
			display = filepath.Join(root, display)
		}
		info, err := os.Stat(display)
		if err != nil || info.Mode()&os.ModeSocket == 0 {
			r.issue(required, "graphics", "Wayland socket is missing, inaccessible or not a socket")
			return
		}
		r.line("WARN", "graphics", "Wayland socket exists; compositor connection and layer-shell protocol support not probed (GNOME/Mutter is unsupported)")
	case "darwin":
		r.line("WARN", "graphics", "macOS native overlay requires a logged-in graphical session; WindowServer access and displays not probed")
	case "windows":
		r.line("WARN", "graphics", "native overlay requires an interactive desktop and Windows 10 1803+; OS version and desktop access not probed")
	default:
		r.issue(required, "graphics", "native overlay is unsupported on this platform")
	}
}

func (r *doctorReport) audio(required bool) {
	switch runtime.GOOS {
	case "linux":
		pulse := os.Getenv("PULSE_SERVER") != ""
		if root := os.Getenv("XDG_RUNTIME_DIR"); root != "" {
			info, err := os.Stat(filepath.Join(root, "pulse", "native"))
			pulse = pulse || err == nil && info.Mode()&os.ModeSocket != 0
		}
		_, alsaErr := os.Stat("/dev/snd")
		if !pulse && alsaErr != nil {
			r.line("WARN", "audio", "no PulseAudio endpoint hint or /dev/snd found; custom audio configuration may still work")
		} else {
			r.line("WARN", "audio", "PulseAudio endpoint hint or ALSA device directory found; server/device access and default output not probed")
		}
	case "darwin":
		r.line("WARN", "audio", "CoreAudio output devices and permissions not probed")
	case "windows":
		r.line("WARN", "audio", "Windows audio service, output devices and permissions not probed")
	default:
		r.issue(required, "audio", "TTS audio support is unverified on this platform")
	}
	r.line("WARN", "tts network", "speech synthesis requires network access; endpoint and proxy connectivity not probed")
}
