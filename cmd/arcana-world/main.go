// Arcana World 是采用 GPL-3.0-only 的 Go/TUI 直播控制台。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	"arcana-world/internal/journal"
	"arcana-world/internal/overlay"
	"arcana-world/internal/store"
	"arcana-world/internal/tui"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var version = "0.4.4"

var logger = journal.Logger("main")

// logRunFailure 记录 runArgs 返回的错误；调用时日志必须仍处于打开状态。
func logRunFailure(err *error) {
	if *err != nil {
		logger.Error("application failed", "err", *err)
	}
}

type startupInfo struct {
	dir, logPath, logLevel, credentialBackend string
	useWeb                                    bool
	configErr                                 error
	proxy                                     *string
	obsAutoConnect, obsAutoStream             *bool
	overlay, tts                              *bool
	webPort                                   int
}

// optionalBool 区分命令行覆盖值与未指定。
func optionalBool(v *bool) any {
	if v == nil {
		return "unset"
	}
	return *v
}

// redactedProxy 只记录脱敏后的代理地址，绝不输出用户名或密码。
func redactedProxy(proxy *string) string {
	if proxy == nil {
		return "unset"
	}
	if *proxy == "" {
		return "none"
	}
	u, err := url.Parse(*proxy)
	if err != nil {
		return "invalid"
	}
	return u.Redacted()
}

func logStarted(info startupInfo) {
	ui := "tui"
	if info.useWeb {
		ui = "webui"
	}
	attrs := []any{
		"version", version, "goos", runtime.GOOS, "goarch", runtime.GOARCH, "go_version", runtime.Version(), "pid", os.Getpid(),
		"data_dir", info.dir, "log_path", info.logPath, "log_level", info.logLevel, "language", i18n.Language(), "ui", ui,
		"credential_backend", info.credentialBackend, "override_proxy", redactedProxy(info.proxy),
		"override_obs_auto_connect", optionalBool(info.obsAutoConnect), "override_obs_auto_stream", optionalBool(info.obsAutoStream),
		"override_overlay", optionalBool(info.overlay), "override_tts", optionalBool(info.tts),
	}
	if info.useWeb {
		attrs = append(attrs, "web_port", info.webPort)
	}
	if info.configErr != nil {
		attrs = append(attrs, "config_err", info.configErr)
	}
	logger.Info("application started", attrs...)
}

func run() (err error) {
	return runArgs(os.Args[1:])
}

func runArgs(args []string) (err error) {
	flags := flag.NewFlagSet("arcana-world", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	dir := flags.String("config-dir", "", "")
	showVersion := flags.Bool("version", false, "")
	language := flags.String("lang", "auto", "")
	enableOverlay := flags.Bool("overlay", false, "")
	overlayExecutable := flags.String("overlay-executable", "", "")
	credentialBackend := flags.String("credential-backend", "auto", "")
	proxy := flags.String("proxy", "", "")
	obsAutoConnect := flags.Bool("obs-auto-connect", false, "")
	obsAutoStream := flags.Bool("obs-auto-stream", false, "")
	enableTTS := flags.Bool("tts", false, "")
	doctor := flags.Bool("doctor", false, "")
	checkUpdate := flags.Bool("check-update", false, "")
	enableTUI := flags.Bool("tui", false, "")
	enableGUI := flags.Bool("gui", false, "")
	defaultTUI := flags.Bool("default-tui", false, "")
	defaultGUI := flags.Bool("default-gui", false, "")
	webPort := flags.Int("web-port", 5273, "")
	logLevel := flags.String("log-level", "info", "")
	help := flags.Bool("help", false, "")
	shortHelp := flags.Bool("h", false, "")
	parseErr := flags.Parse(args)
	if err := i18n.SetLanguage(*language); err != nil {
		return err
	}
	flags.SetOutput(os.Stderr)
	flags.Lookup("config-dir").Usage = i18n.T(i18n.CLIConfigDirUsage)
	flags.Lookup("version").Usage = i18n.T(i18n.CLIVersionUsage)
	flags.Lookup("lang").Usage = i18n.T(i18n.CLILanguageUsage)
	flags.Lookup("overlay").Usage = i18n.T(i18n.CLIOverlayUsage)
	flags.Lookup("overlay-executable").Usage = i18n.T(i18n.CLIOverlayExecutableUsage)
	flags.Lookup("credential-backend").Usage = i18n.T(i18n.CLICredentialBackendUsage)
	flags.Lookup("proxy").Usage = i18n.T(i18n.CLIProxyUsage)
	flags.Lookup("obs-auto-connect").Usage = i18n.T(i18n.CLIOBSAutoConnectUsage)
	flags.Lookup("obs-auto-stream").Usage = i18n.T(i18n.CLIOBSAutoStreamUsage)
	flags.Lookup("tts").Usage = i18n.T(i18n.CLITTSUsage)
	flags.Lookup("doctor").Usage = i18n.T(i18n.CLIDoctorUsage)
	flags.Lookup("check-update").Usage = i18n.T(i18n.CLIUpdateUsage)
	flags.Lookup("tui").Usage = i18n.T(i18n.CLITUIUsage)
	flags.Lookup("gui").Usage = i18n.T(i18n.CLIGUIUsage)
	flags.Lookup("default-tui").Usage = i18n.T(i18n.CLIDefaultTUIUsage)
	flags.Lookup("default-gui").Usage = i18n.T(i18n.CLIDefaultGUIUsage)
	flags.Lookup("web-port").Usage = i18n.T(i18n.CLIWebPortUsage)
	flags.Lookup("log-level").Usage = i18n.T(i18n.CLILogLevelUsage)
	flags.Lookup("help").Usage = i18n.T(i18n.CLIHelpUsage)
	flags.Lookup("h").Usage = flags.Lookup("help").Usage
	flags.Usage = func() {
		fmt.Fprint(flags.Output(), i18n.T(i18n.CLIHelp))
		flags.PrintDefaults()
	}
	if parseErr != nil {
		return fmt.Errorf("%s: %w", i18n.T(i18n.CLIInvalidArguments), parseErr)
	}
	if *help || *shortHelp {
		flags.Usage()
		return nil
	}
	if *showVersion {
		fmt.Printf("arcana-world %s\n%s\n", version, i18n.T(i18n.CLILicense))
		return nil
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("%s", i18n.T(i18n.CLIPositionalArgumentsUnsupported))
	}
	if *checkUpdate && *doctor {
		return errors.New(i18n.T(i18n.CLIUpdateConflict))
	}
	modeCount := 0
	for _, selected := range []bool{*enableTUI, *enableGUI, *defaultTUI, *defaultGUI} {
		if selected {
			modeCount++
		}
	}
	if modeCount > 1 || (modeCount > 0 && (*doctor || *checkUpdate)) {
		return errors.New(i18n.T(i18n.CLIUIModeConflict))
	}
	switch *credentialBackend {
	case "auto", "system", "file", "memory":
	default:
		return errors.New(i18n.T(i18n.CLICredentialBackendInvalid))
	}
	switch *logLevel {
	case "debug":
		journal.SetLevel(slog.LevelDebug)
	case "info":
		journal.SetLevel(slog.LevelInfo)
	case "warn":
		journal.SetLevel(slog.LevelWarn)
	case "error":
		journal.SetLevel(slog.LevelError)
	default:
		return errors.New(i18n.T(i18n.CLILogLevelInvalid))
	}
	options := store.Options{CredentialBackend: *credentialBackend}
	var overlayOverride, ttsOverride *bool
	webPortSet := false
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "web-port":
			webPortSet = true
		case "proxy":
			options.Overrides.Proxy = proxy
		case "obs-auto-connect":
			options.Overrides.OBSAutoConnect = obsAutoConnect
		case "obs-auto-stream":
			options.Overrides.OBSAutoStream = obsAutoStream
		case "overlay":
			overlayOverride = enableOverlay
		case "tts":
			ttsOverride = enableTTS
		}
	})
	if (*doctor || *checkUpdate) && webPortSet {
		return errors.New(i18n.T(i18n.CLIUIModeConflict))
	}
	if *defaultTUI || *defaultGUI {
		conflict := false
		flags.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "config-dir", "lang", "credential-backend", "default-tui", "default-gui":
			default:
				conflict = true
			}
		})
		if conflict {
			return errors.New(i18n.T(i18n.CLIUIModeConflict))
		}
		ui := "tui"
		if *defaultGUI {
			ui = "webui"
		}
		if err := store.SaveDefaultUI(*dir, ui); err != nil {
			return err
		}
		fmt.Printf(i18n.T(i18n.CLIDefaultUISaved), ui)
		return nil
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *checkUpdate {
		return runUpdateCheck(ctx, os.Stdout, *dir, options)
	}
	if *webPort < 0 || *webPort > 65535 {
		return errors.New(i18n.T(i18n.WebPortInvalid))
	}
	if options.Overrides.Proxy != nil {
		client, err := bili.New(*proxy, "")
		if err != nil {
			return err
		}
		client.HTTP.CloseIdleConnections()
	}
	if *doctor {
		return runDoctor(ctx, os.Stdout, *dir, options, *overlayExecutable, overlayOverride, ttsOverride)
	}
	resolvedDir, err := store.ResolveDir(*dir)
	if err != nil {
		return err
	}
	// 读取启动配置前锁定目录，避免默认项修改与实例启动交叉覆盖。
	disk, err := journal.Open(resolvedDir)
	if err != nil {
		return err
	}
	defer func() {
		if disk != nil {
			err = errors.Join(err, disk.Close())
		}
	}()
	// 模型尚未接管日志时的失败记录；接管后日志已关闭，记录会被丢弃，由模型之后的同名 defer 负责。
	defer logRunFailure(&err)
	startupConfig, err := store.ReadConfig(resolvedDir)
	useWeb := err == nil && startupConfig.DefaultUI == "webui"
	if *enableTUI {
		useWeb = false
	}
	if *enableGUI {
		useWeb = true
	}
	logStarted(startupInfo{
		dir: resolvedDir, logPath: disk.Path(), logLevel: *logLevel, useWeb: useWeb, configErr: err, credentialBackend: *credentialBackend,
		proxy: options.Overrides.Proxy, obsAutoConnect: options.Overrides.OBSAutoConnect, obsAutoStream: options.Overrides.OBSAutoStream,
		overlay: overlayOverride, tts: ttsOverride, webPort: *webPort,
	})
	if err != nil {
		return err
	}
	if webPortSet && !useWeb {
		return errors.New(i18n.T(i18n.WebPortRequiresWeb))
	}
	s, err := store.Open(resolvedDir, options)
	if err != nil {
		return err
	}
	model, err := tui.New(ctx, s, disk)
	disk = nil // New 接管日志及目录锁，包括初始化失败时的释放。
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, model.Close()) }()
	// 后注册先执行：必须在 model.Close 关闭日志之前写入失败原因。
	defer logRunFailure(&err)
	if ttsOverride != nil {
		if err := model.ConfigureTTS(*ttsOverride); err != nil {
			return err
		}
	}
	cfg := s.Config()
	effectiveOverlay := cfg.Overlay.Enabled
	if overlayOverride != nil {
		effectiveOverlay = *overlayOverride
	}
	if err := model.ConfigureOverlay(overlay.Options{Executable: *overlayExecutable, Config: cfg.Overlay.Config("")}, effectiveOverlay); err != nil {
		return err
	}
	if useWeb {
		return runWeb(ctx, model, cfg.WebBindAddress, *webPort)
	}
	defer func() { _, _ = fmt.Fprint(os.Stdout, ansi.SetPointerShape("default")) }()
	// 进程信号统一由上层上下文接管，避免事件循环退出后仍有信号处理器等待投递。
	_, err = tea.NewProgram(model, tea.WithContext(ctx), tea.WithoutSignalHandler()).Run()
	if ctx.Err() != nil {
		return nil
	}
	return err
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "arcana-world:", err)
		os.Exit(1)
	}
}
