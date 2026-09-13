// Arcana World is a GPL-3.0 Go/TUI adaptation of Radekyspec/StartLive.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"arcana-world/internal/store"
	"arcana-world/internal/tui"
	tea "github.com/charmbracelet/bubbletea"
)

var version = "0.1.3"

func run() (err error) {
	dir := flag.String("config-dir", "", "数据目录（默认：~/.arcana/world）")
	showVersion := flag.Bool("version", false, "显示版本与许可证")
	flag.Usage = func() {
		fmt.Fprint(flag.CommandLine.Output(), "Arcana World — B 站直播控制台 / Bubble Tea\n\n用法: arcana-world [选项]\n\n账号 cookies 与 OBS 密码使用系统密钥环，不写入明文配置。\nLinux 需要已解锁的 Secret Service；OBS 需要 WebSocket v5。\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()
	if *showVersion {
		fmt.Printf("arcana-world %s\nGPL-3.0 · Based on Radekyspec/StartLive\n", version)
		return nil
	}
	if flag.NArg() != 0 {
		return fmt.Errorf("不支持的位置参数；使用 --help 查看用法")
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
