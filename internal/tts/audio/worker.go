//go:build tts

package audio

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"arcana-world/internal/tts/meloproto"
	"github.com/ebitengine/oto/v3"
)

const workerSampleRate = 48000

// RunWorker 复用一个设备上下文，空闲时挂起设备并阻塞读取下一帧。
// Oto 没有关闭上下文的 API；返回后专用进程必须退出以释放设备。
func RunWorker(ctx context.Context, in io.Reader, out io.Writer, volume int) (err error) {
	if ctx == nil || in == nil || out == nil {
		return errors.New("audio: nil worker context or stream")
	}
	if volume < 0 || volume > 100 {
		return errors.New("audio: volume must be between 0 and 100")
	}
	var device *oto.Context
	defer func() {
		if device != nil {
			err = errors.Join(err, device.Suspend(), device.Err())
		}
	}()
	if err := meloproto.Write(out, []byte("audio-play-v1")); err != nil {
		return err
	}
	for {
		data, err := meloproto.Read(in, maxAudioBytes)
		if err == io.EOF {
			return ctx.Err()
		}
		if err != nil {
			// 收到信号时标准输入会被关闭，此时报告取消而不是读取错误。
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		decoder, err := decodeAudio(data)
		if err != nil {
			return err
		}
		if device == nil {
			var ready chan struct{}
			device, ready, err = oto.NewContext(&oto.NewContextOptions{
				SampleRate: workerSampleRate, ChannelCount: 2,
				Format: oto.FormatFloat32LE, BufferSize: playbackBufferDuration,
			})
			if err != nil {
				return fmt.Errorf("audio: initialize device: %w", err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ready:
			}
		}
		if err := device.Err(); err != nil {
			return fmt.Errorf("audio: device: %w", err)
		}
		if err := device.Resume(); err != nil {
			return fmt.Errorf("audio: resume device: %w", err)
		}
		err = playWorkerAudio(ctx, device, decoder, volume)
		err = errors.Join(err, device.Suspend(), device.Err())
		if err != nil {
			return err
		}
		if err := meloproto.Write(out, []byte("played")); err != nil {
			return err
		}
	}
}

func playWorkerAudio(ctx context.Context, device *oto.Context, decoder *decodedAudio, volume int) error {
	ctx, cancel := context.WithTimeout(ctx, (maxAudioSeconds+10)*time.Second)
	defer cancel()
	source := &trackedPCM{source: decoder.source, ctx: ctx, limit: decoder.rate * decoder.channels * decoder.sampleBytes * maxAudioSeconds}
	decoder.source = source
	stream := &trackedPCM{source: newWorkerPCM(decoder), ctx: ctx, limit: workerSampleRate * 8 * maxAudioSeconds}
	player := device.NewPlayer(stream)
	player.SetBufferSize(int(playbackBufferDuration*workerSampleRate/time.Second) * 8)
	player.SetVolume(float64(volume) / 100)
	player.Play()
	return waitPlayback(ctx, player, device.Err, stream, playbackDrainTail)
}
