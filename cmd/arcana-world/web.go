package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	fmt.Printf(i18n.T(i18n.WebStarted), server.URL())
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
