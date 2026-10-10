package main

import (
	"context"
	"errors"

	"arcana-world/internal/bot"
	"arcana-world/internal/i18n"
	"arcana-world/internal/store"
)

func runBot(ctx context.Context, dir string, options store.Options) (err error) {
	input, output, err := bot.Stdio()
	if err != nil {
		return errors.New(i18n.T(i18n.CLIBotFailed))
	}
	defer input.Close()
	defer output.Close()
	s, err := store.Open(dir, options)
	if err != nil {
		return errors.New(i18n.T(i18n.CLIBotFailed))
	}
	session, err := bot.NewSession(ctx, s, version)
	if err != nil {
		return errors.New(i18n.T(i18n.CLIBotFailed))
	}
	defer func() {
		if closeErr := session.Close(); closeErr != nil {
			err = errors.New(i18n.T(i18n.CLIBotFailed))
		}
	}()
	if err := bot.Serve(ctx, input, output, session); err != nil && ctx.Err() == nil {
		return errors.New(i18n.T(i18n.CLIBotFailed))
	}
	return nil
}
