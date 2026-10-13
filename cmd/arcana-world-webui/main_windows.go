package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"unsafe"

	"arcana-world/internal/i18n"

	"golang.org/x/sys/windows"
)

func main() {
	_ = i18n.SetLanguage("auto")
	if err := run(os.Args[1:]); err != nil {
		message := fmt.Sprintf(i18n.T(i18n.CLIWebUILauncherFailed), err.Error())
		fmt.Fprintln(os.Stderr, message)
		showError(message)
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func run(args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	command := webUICommand(executable, args)
	// 不另建控制台或进程组；子进程继承工作目录，并直接收到控制台关闭及中断事件。
	// 启动器等待子进程完成清理，避免 Ctrl+C 先结束启动器而丢失退出结果。
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(interrupts)
	return command.Run()
}

func webUICommand(executable string, args []string) *exec.Cmd {
	// 最后设置 GUI，避免透传的 --gui=false 关闭启动器的临时界面覆盖。
	// 若用户用 -- 或位置参数提前结束解析，主程序会拒绝剩余参数而非启动其他界面。
	commandArgs := make([]string, len(args)+1)
	copy(commandArgs, args)
	commandArgs[len(args)] = "--gui"
	command := exec.Command(filepath.Join(filepath.Dir(executable), "arcana-world.exe"), commandArgs...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	return command
}

func showError(message string) {
	// 双击启动时控制台会随进程关闭，错误对话框保证失败信息仍可阅读。
	text, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	title, _ := windows.UTF16PtrFromString("Arcana World WebUI")
	messageBox := windows.NewLazySystemDLL("user32.dll").NewProc("MessageBoxW")
	if err := messageBox.Find(); err != nil {
		return
	}
	messageBox.Call(0, uintptr(unsafe.Pointer(text)), uintptr(unsafe.Pointer(title)), 0x10)
}
