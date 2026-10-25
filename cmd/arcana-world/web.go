package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"time"

	"arcana-world/internal/i18n"
	"arcana-world/internal/tui"
	"arcana-world/internal/webui"
	tea "charm.land/bubbletea/v2"
)

func runWeb(ctx context.Context, model *tui.Model, bindAddress string, port int) (err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	model.EnableWeb()
	program := tea.NewProgram(model, tea.WithContext(ctx), tea.WithoutRenderer(), tea.WithInput(nil), tea.WithOutput(io.Discard), tea.WithoutSignalHandler())
	server, err := webui.Listen(bindAddress, port, func(requestCtx context.Context, page string, command *webui.Command) (webui.Snapshot, error) {
		requestCtx, requestCancel := context.WithCancel(requestCtx)
		stop := context.AfterFunc(ctx, requestCancel)
		defer stop()
		defer requestCancel()
		return model.WebRequest(requestCtx, program.Send, page, command)
	})
	if err != nil {
		return err
	}
	defer func() {
		shutdownCtx, done := context.WithTimeout(context.Background(), 3*time.Second)
		defer done()
		err = errors.Join(err, server.Close(shutdownCtx))
	}()
	served := make(chan error, 1)
	go func() {
		serveErr := server.Serve()
		served <- serveErr
		cancel()
	}()
	logger.Info("web server started", "bind", bindAddress, "port", port, "url", server.URL())
	fmt.Printf(i18n.T(i18n.WebStarted), server.URL())
	browserDone := openBrowser(ctx, server.URL())
	defer func() {
		cancel()
		<-browserDone
	}()
	_, err = program.Run()
	if ctx.Err() != nil {
		err = nil
	}
	cancel()
	select {
	case serveErr := <-served:
		err = errors.Join(err, serveErr)
	default:
	}
	return err
}

func openBrowser(ctx context.Context, url string) <-chan struct{} {
	done := make(chan struct{})
	var command *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		// 直接调用系统处理程序，不经过 cmd.exe 解释 URL。
		command = exec.CommandContext(ctx, "rundll32.exe", "url.dll,FileProtocolHandler", url)
	case "darwin":
		command = exec.CommandContext(ctx, "open", url)
	default:
		command = exec.CommandContext(ctx, "xdg-open", url)
	}
	if err := command.Start(); err != nil {
		logger.Warn("browser open failed", "command", command.Path, "url", url, "err", err)
		fmt.Fprintf(os.Stderr, i18n.T(i18n.WebBrowserOpenFailed), url)
		close(done)
		return done
	}
	// 浏览器可能保持运行；异步等待以回收子进程，不阻塞界面事件循环。
	go func() {
		defer close(done)
		if err := command.Wait(); err != nil && ctx.Err() == nil {
			logger.Warn("browser exited with error", "command", command.Path, "url", url, "err", err)
			fmt.Fprintf(os.Stderr, i18n.T(i18n.WebBrowserOpenFailed), url)
		}
	}()
	return done
}
