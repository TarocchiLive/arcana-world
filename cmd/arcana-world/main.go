// Arcana World 是采用 GPL-3.0-only 的 Go/TUI 直播控制台。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	"arcana-world/internal/store"
	"arcana-world/internal/tui"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

var version = "0.3.0-alpha.3"

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
	switch *credentialBackend {
	case "auto", "system", "file", "memory":
	default:
		return errors.New(i18n.T(i18n.CLICredentialBackendInvalid))
	}
	options := store.Options{CredentialBackend: *credentialBackend}
	var overlayOverride, ttsOverride *bool
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
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
	if options.Overrides.Proxy != nil {
		client, err := bili.New(*proxy)
		if err != nil {
			return err
		}
		client.HTTP.CloseIdleConnections()
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *doctor {
		return runDoctor(ctx, os.Stdout, *dir, options, *overlayExecutable, overlayOverride, ttsOverride)
	}
	s, err := store.Open(*dir, options)
	if err != nil {
		return err
	}
	model, err := tui.New(ctx, s)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, model.Close()) }()
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
	defer func() { _, _ = fmt.Fprint(os.Stdout, ansi.SetPointerShape("default")) }()
	_, err = tea.NewProgram(model, tea.WithContext(ctx)).Run()
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
