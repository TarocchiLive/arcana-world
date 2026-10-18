// arcana-world-overlay 是独立于 TUI 的桌面文字浮层。
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
)

func init() { runtime.LockOSThread() }

// startupContext 在参数解析后填入运行模式与显示器选择，供致命错误诊断使用。
var startupContext string

func main() {
	if err := run(); err != nil {
		// 父进程把本进程 stderr 收入日志；输出单行英文诊断，包含后端、显示器与原因。
		line := "arcana-world-overlay: fatal: backend=" + string(backendID)
		if startupContext != "" {
			line += " " + startupContext
		}
		line += " err=" + strings.Join(strings.Fields(err.Error()), " ")
		fmt.Fprintln(os.Stderr, line)
		os.Exit(1)
	}
}
func run() error {
	flags := flag.NewFlagSet("arcana-world-overlay", flag.ContinueOnError)
	cfg := overlay.DefaultConfig()
	cfg.Backend = backendID
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
	demo := flags.Bool("demo", false, "continuously cycle sample chat messages")
	reducedMotion := flags.Bool("reduced-motion", false, "disable Elegant overlay motion")
	unlocked := flags.Bool("unlocked", false, "allow dragging the Elegant overlay")
	flags.BoolVar(&cfg.Qt.ShowInTaskbar, "show-in-taskbar", false, "show Elegant overlay in macOS Dock or Windows taskbar; inactive on Linux; does not guarantee capture")
	registerBackendFlags(flags, &cfg)
	duration := flags.Duration("duration", 0, "close automatically after this duration; 0 waits for Ctrl+C")
	socket := flags.String("socket", "", "managed host Unix socket; cannot be combined with standalone options")
	flags.Usage = func() {
		fmt.Fprintln(flags.Output(), "Usage: arcana-world-overlay [options]")
		fmt.Fprintln(flags.Output(), backendHelp)
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
	mode := "standalone"
	switch {
	case *socket != "":
		mode = "managed"
	case *listDisplays:
		mode = "list-displays"
	}
	if mode == "managed" {
		startupContext = "mode=managed" // 显示器选择由父进程经套接字下发。
	} else {
		startupContext = fmt.Sprintf("mode=%s display=%d output=%q displays=%q", mode, *display, cfg.Output, cfg.Displays)
	}
	var explicitDisplays, legacyDisplay, explicitTaskbar bool
	flags.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "displays":
			explicitDisplays = true
		case "display", "output":
			legacyDisplay = true
		case "show-in-taskbar":
			explicitTaskbar = true
		}
	})
	if explicitTaskbar && backendID != overlay.BackendQt {
		return errors.New("-show-in-taskbar requires the Elegant overlay helper")
	}
	if explicitDisplays && legacyDisplay {
		return errors.New("-displays cannot be combined with -display or -output")
	}
	if explicitDisplays && cfg.Displays == "" {
		return errors.New("-displays requires a JSON array; use [] to hide all displays")
	}
	if err := configurePortableData(); err != nil {
		return err
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
			if cfg.Backend != backendID {
				return errors.New("display selection backend does not match this helper")
			}
		}
		displays, err := listBackendDisplays(ctx, cfg)
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
		return overlay.Serve(ctx, *socket, backendID, runBackend)
	}
	if uint64(*display) > uint64(^uint32(0)) {
		return errors.New("-display must fit in an unsigned 32-bit integer")
	}
	if *duration < 0 {
		return errors.New("-duration must not be negative")
	}
	if (*clock && *stdin) || (*demo && (*clock || *stdin)) {
		return errors.New("-clock, -stdin and -demo are mutually exclusive")
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
	if err := configureBackend(*reducedMotion, *unlocked); err != nil {
		return err
	}
	if *demo {
		cfg.Text = demoLines[0]
		cfg.TextRoles = "n"
	}
	if *duration > 0 {
		var stop context.CancelFunc
		ctx, stop = context.WithTimeout(ctx, *duration)
		defer stop()
	}
	var updates chan overlay.Config
	inputError := make(chan error, 1)
	if *clock || *stdin || *demo {
		updates = make(chan overlay.Config)
	}
	if *demo {
		go streamDemo(ctx, cfg, updates)
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
	err = runBackend(ctx, cfg, updates, func() { fmt.Fprintln(os.Stderr, "Arcana World Overlay: ready; Ctrl+C to close") })
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
