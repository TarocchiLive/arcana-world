package tts

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"time"
)

type PlayerOptions struct {
	Executable string
	Timeout    time.Duration
	Volume     int // 0 表示静音播放；调用方必须提供所需音量。
}

// ProcessPlayer 通过常驻辅助进程逐个播放 MP3、WAV、FLAC 或 Ogg Vorbis。
type ProcessPlayer struct {
	executable string
	timeout    time.Duration
	volume     int
	client     *residentClient
}

func NewPlayer(options PlayerOptions) (*ProcessPlayer, error) {
	if options.Volume < 0 || options.Volume > 100 {
		return nil, errors.New("tts: playback volume must be between 0 and 100")
	}
	if options.Timeout < 0 {
		return nil, errors.New("tts: negative playback timeout")
	}
	if options.Timeout == 0 {
		options.Timeout = 130 * time.Second
	}
	executable, err := resolveHelper("arcana-world-tts", options.Executable)
	if err != nil {
		return nil, err
	}
	logger.Info("player ready", "executable", executable, "volume", options.Volume, "timeout", options.Timeout)
	return &ProcessPlayer{executable: executable, timeout: options.Timeout, volume: options.Volume, client: newResidentClient()}, nil
}

func (p *ProcessPlayer) Play(ctx context.Context, audio []byte) error {
	if ctx == nil {
		return errors.New("nil playback context")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(audio) == 0 || len(audio) > MaxAudioBytes {
		return fmt.Errorf("audio must contain between 1 byte and %d MiB", MaxAudioBytes>>20)
	}
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	// 只有收到播放完成确认才返回，空闲期间辅助进程不播放静音。
	if _, err := p.client.request(ctx, func(context.Context) (*exec.Cmd, error) {
		return exec.Command(p.executable, "--volume", strconv.Itoa(p.volume), "play-worker"), nil
	}, "audio-play-v1", audio, len("played"), func(data []byte) bool { return string(data) == "played" }); err != nil {
		return err
	}
	return nil
}

func (p *ProcessPlayer) Close() error { return p.client.Close() }
