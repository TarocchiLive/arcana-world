// arcana-world-overlay 是可选的原生桌面文字浮层，独立于 TUI。
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"arcana-world/internal/overlay"
	"arcana-world/internal/overlay/native"
)

func init() { runtime.LockOSThread() }
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "arcana-world-overlay:", err)
		os.Exit(1)
	}
}
func run() error {
	flags := flag.NewFlagSet("arcana-world-overlay", flag.ContinueOnError)
	cfg := overlay.DefaultConfig()
	flags.StringVar(&cfg.Text, "text", cfg.Text, "text to display")
	anchor := flags.String("anchor", string(cfg.Position.Anchor), "top-left, top, top-right, left, center, right, bottom-left, bottom, bottom-right")
	flags.Float64Var(&cfg.Position.X, "x", cfg.Position.X, "horizontal anchor offset in logical points; positive points inward at edges")
	flags.Float64Var(&cfg.Position.Y, "y", cfg.Position.Y, "vertical anchor offset in logical points; positive points inward at edges")
	flags.Float64Var(&cfg.Width, "width", cfg.Width, "panel width in logical points")
	flags.Float64Var(&cfg.Height, "height", cfg.Height, "panel height in logical points")
	padding := flags.String("padding", fmt.Sprintf("%g,%g,%g,%g", cfg.Padding.Top, cfg.Padding.Right, cfg.Padding.Bottom, cfg.Padding.Left), "padding: one value, or top,right,bottom,left in logical points")
	flags.StringVar(&cfg.Font.Family, "font", cfg.Font.Family, "font family; empty uses the platform default")
	flags.Float64Var(&cfg.Font.Size, "font-size", cfg.Font.Size, "font size in logical points")
	flags.IntVar(&cfg.Font.Weight, "font-weight", cfg.Font.Weight, "font weight (100..900)")
	flags.BoolVar(&cfg.Font.Italic, "italic", false, "use italic font")
	flags.BoolVar(&cfg.Outline, "outline", cfg.Outline, "draw a thin text outline")
	flags.Float64Var(&cfg.TextAlpha, "text-alpha", cfg.TextAlpha, "text opacity (0..1)")
	flags.Float64Var(&cfg.BackgroundAlpha, "background-alpha", cfg.BackgroundAlpha, "background opacity (0..1)")
	display := flags.Uint("display", 0, "macOS display ID or one-based Windows monitor index; 0 selects the default")
	flags.StringVar(&cfg.Output, "output", "", "Wayland output name or Windows display device name; alternative to -display")
	flags.StringVar(&cfg.Displays, "displays", "", "JSON array of stable display IDs; [] hides all; cannot combine with -display/-output")
	listDisplays := flags.Bool("list-displays", false, "print connected display IDs and names as JSON")
	displayConfigStdin := flags.Bool("display-config-stdin", false, "read display selection config from stdin; requires -list-displays")
	clock := flags.Bool("clock", false, "append a live clock to demonstrate updates")
	stdin := flags.Bool("stdin", false, "replace text with each input line; EOF retains the last line")
	duration := flags.Duration("duration", 0, "close automatically after this duration; 0 waits for Ctrl+C")
	socket := flags.String("socket", "", "managed host Unix socket; cannot be combined with standalone options")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: arcana-world-overlay [options]")
		fmt.Fprintln(flags.Output(), "Native click-through overlay: macOS, Windows 10 1803+, KDE/niri Wayland.")
		fmt.Fprintln(flags.Output(), "Linux: make overlay requires cgo, -tags wayland, wayland-client and pangocairo.")
		fmt.Fprintln(flags.Output(), "GNOME/Mutter lacks layer-shell. No ordinary-window or X11 fallback.")
		flags.PrintDefaults()
	}
	if err := flags.Parse(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("positional arguments are not supported")
	}
	var explicitDisplays, legacyDisplay bool
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "displays":
			explicitDisplays = true
		case "display", "output":
			legacyDisplay = true
		}
	})
	if explicitDisplays && legacyDisplay {
		return errors.New("-displays cannot be combined with -display or -output")
	}
	if explicitDisplays && cfg.Displays == "" {
		return errors.New("-displays requires a JSON array; use [] to hide all displays")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if *listDisplays {
		if flags.NFlag() != 1 && !(flags.NFlag() == 2 && *displayConfigStdin) {
			return errors.New("-list-displays cannot be combined with other options")
		}
		if *displayConfigStdin {
			data, err := io.ReadAll(io.LimitReader(os.Stdin, 512*1024+1))
			if err != nil {
				return fmt.Errorf("display selection: %w", err)
			}
			if len(data) > 512*1024 {
				return errors.New("display selection exceeds 512 KiB")
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				return fmt.Errorf("display selection: %w", err)
			}
			cfg, err = cfg.Normalize()
			if err != nil {
				return err
			}
		}
		displays, err := native.ListDisplays(ctx, cfg)
		if err != nil {
			return err
		}
		if displays == nil {
			displays = []overlay.Display{}
		}
		return json.NewEncoder(os.Stdout).Encode(displays)
	}
	if *displayConfigStdin {
		return errors.New("-display-config-stdin requires -list-displays")
	}
	if *socket != "" {
		if flags.NFlag() != 1 {
			return errors.New("-socket cannot be combined with standalone options")
		}
		return overlay.Serve(ctx, *socket, native.Run)
	}
	if uint64(*display) > uint64(^uint32(0)) {
		return errors.New("-display must fit in an unsigned 32-bit integer")
	}
	if *duration < 0 {
		return errors.New("-duration must not be negative")
	}
	if *clock && *stdin {
		return errors.New("-clock and -stdin are mutually exclusive")
	}
	cfg.DisplayID = uint32(*display)
	cfg.Position.Anchor = overlay.Anchor(*anchor)
	var err error
	cfg.Padding, err = parsePadding(*padding)
	if err != nil {
		return err
	}
	cfg, err = cfg.Normalize()
	if err != nil {
		return err
	}
	if *duration > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, *duration)
		defer stop()
	}
	var updates chan overlay.Config
	inputError := make(chan error, 1)
	if *clock || *stdin {
		updates = make(chan overlay.Config)
	}
	if *clock {
		go func() {
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			defer close(updates)
			for {
				select {
				case <-ctx.Done():
					return
				case now := <-ticker.C:
					next := cfg
					next.Text = cfg.Text + "\n" + now.Format("15:04:05")
					select {
					case updates <- next:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}
	if *stdin {
		go func() {
			defer close(updates)
			scanner := bufio.NewScanner(os.Stdin)
			scanner.Buffer(make([]byte, 4096), overlay.MaxTextBytes+2) // 为 Windows 的 CRLF 保留空间。
			for scanner.Scan() {
				next := cfg
				next.Text = scanner.Text()
				select {
				case updates <- next:
				case <-ctx.Done():
					return
				}
			}
			if err := scanner.Err(); err != nil {
				inputError <- fmt.Errorf("stdin: %w", err)
				cancel()
			}
		}()
	}
	err = native.Run(ctx, cfg, updates, func() { fmt.Fprintln(os.Stderr, "Arcana World Overlay: ready; Ctrl+C to close") })
	select {
	case inputErr := <-inputError:
		return inputErr
	default:
		return err
	}
}

func parsePadding(value string) (overlay.Insets, error) {
	parts := strings.Split(value, ",")
	if len(parts) != 1 && len(parts) != 4 {
		return overlay.Insets{}, errors.New("-padding requires one value or top,right,bottom,left")
	}
	var values [4]float64
	for index, part := range parts {
		number, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil {
			return overlay.Insets{}, fmt.Errorf("-padding: %w", err)
		}
		values[index] = number
	}
	if len(parts) == 1 {
		values = [4]float64{values[0], values[0], values[0], values[0]}
	}
	return overlay.Insets{Top: values[0], Right: values[1], Bottom: values[2], Left: values[3]}, nil
}
