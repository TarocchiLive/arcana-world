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

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	"arcana-world/internal/store"
	"arcana-world/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

var version = "0.2.0-rc.1"

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
	s, err := store.Open(*dir)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	model, err := tui.New(ctx, s)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, model.Close()) }()
	cfg := s.Config()
	effectiveOverlay := cfg.Overlay.Enabled
	flags.Visit(func(f *flag.Flag) {
		if f.Name == "overlay" {
			effectiveOverlay = *enableOverlay
		}
	})
	if err := model.ConfigureOverlay(overlay.Options{Executable: *overlayExecutable, Config: cfg.Overlay.Config("")}, effectiveOverlay); err != nil {
		return err
	}
	_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
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
