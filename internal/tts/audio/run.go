//go:build tts_audio

// Package audio implements the one-shot native MP3 playback helper.
package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ebitengine/oto/v3"
)

const (
	playbackBufferDuration = 100 * time.Millisecond
	playbackPollInterval   = 10 * time.Millisecond
	playbackDrainTail      = 500 * time.Millisecond
	playbackBufferBytes    = int(pcmBytesPerSecond * playbackBufferDuration / time.Second)
)

// Run decodes and plays a single MP3. It must only be called once per process:
// Oto does not support creating a second context or closing the first one.
func Run(ctx context.Context, in io.Reader) error {
	if ctx == nil {
		return errors.New("audio: nil context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(in, maxAudioBytes+1))
	if err := ctx.Err(); err != nil {
		return err
	}
	if err != nil {
		return errors.New("audio: reading MP3 input failed")
	}
	decoder, err := decodeMP3(data)
	if err != nil {
		return err
	}
	device, ready, err := oto.NewContext(&oto.NewContextOptions{
		SampleRate:   sampleRate,
		ChannelCount: pcmChannels,
		Format:       oto.FormatSignedInt16LE,
		BufferSize:   playbackBufferDuration,
	})
	if err != nil {
		return fmt.Errorf("audio: initialize device: %w", err)
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-ready:
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := device.Err(); err != nil {
		return fmt.Errorf("audio: device: %w", err)
	}
	stream := &trackedPCM{source: decoder}
	player := device.NewPlayer(stream)
	player.SetBufferSize(playbackBufferBytes)
	player.Play()
	return waitPlayback(ctx, player, device.Err, stream, playbackDrainTail)
}

type playbackDevice interface {
	IsPlaying() bool
	BufferedSize() int
	Err() error
	Close() error
}

func playbackDrained(player playbackDevice, deviceErr func() error, state pcmState) (bool, error) {
	if state.err != nil {
		return false, fmt.Errorf("audio: decode PCM: %w", state.err)
	}
	if err := player.Err(); err != nil {
		return false, fmt.Errorf("audio: player: %w", err)
	}
	if err := deviceErr(); err != nil {
		return false, fmt.Errorf("audio: device: %w", err)
	}
	if !state.eof || player.IsPlaying() || player.BufferedSize() != 0 {
		return false, nil
	}
	if state.bytes == 0 {
		return false, errors.New("audio: MP3 produced no PCM")
	}
	return true, nil
}

// The tail allows queued device audio to finish on the verified target sink.
// Oto has no hardware drain callback: this is not a universal physical drain guarantee.
func waitPlayback(ctx context.Context, player playbackDevice, deviceErr func() error, stream *trackedPCM, tail time.Duration) (err error) {
	defer func() { err = errors.Join(err, player.Close()) }()
	ticker := time.NewTicker(playbackPollInterval)
	defer ticker.Stop()
	var drainedAt time.Time
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		drained, err := playbackDrained(player, deviceErr, stream.snapshot())
		if err != nil {
			return err
		}
		if drained {
			if drainedAt.IsZero() {
				drainedAt = time.Now()
			}
			if time.Since(drainedAt) >= tail {
				return ctx.Err()
			}
		} else {
			drainedAt = time.Time{}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
