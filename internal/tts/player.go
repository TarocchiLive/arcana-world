package tts

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type PlayerOptions struct {
	Executable string
	Timeout    time.Duration
}

// ProcessPlayer plays one MP3 at a time through an isolated audio helper.
type ProcessPlayer struct {
	executable string
	timeout    time.Duration
}

func NewPlayer(options PlayerOptions) (*ProcessPlayer, error) {
	if options.Timeout < 0 {
		return nil, errors.New("tts: negative playback timeout")
	}
	if options.Timeout == 0 {
		options.Timeout = 2 * time.Minute
	}
	executable, err := resolveHelper("arcana-world-tts", options.Executable)
	if err != nil {
		return nil, err
	}
	return &ProcessPlayer{executable: executable, timeout: options.Timeout}, nil
}

func (p *ProcessPlayer) Play(ctx context.Context, audio []byte) error {
	if ctx == nil {
		return errors.New("tts: nil playback context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(audio) == 0 || len(audio) > MaxAudioBytes {
		return errors.New("tts: audio must contain between 1 byte and 8 MiB")
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	if _, err := runHelper(ctx, p.executable, []string{"play"}, audio, 0, nil); err != nil {
		return fmt.Errorf("tts: playback helper: %w", err)
	}
	return nil
}
