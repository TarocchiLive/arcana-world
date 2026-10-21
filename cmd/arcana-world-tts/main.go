//go:build tts

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"arcana-world/internal/tts"
	"arcana-world/internal/tts/audio"
)

func main() {
	volume := flag.Int("volume", 80, "playback volume from 0 (mute) to 100")
	modelDir := flag.String("model-dir", "", "verified local model directory (worker only)")
	provider := flag.String("provider", "melo", "local provider: melo, kokoro or piper (worker only)")
	speaker := flag.Int("speaker", 0, "speaker ID: Kokoro 0..102; Melo/Piper 0 (worker only)")
	threads := flag.Int("threads", tts.DefaultLocalThreads, "CPU inference threads, 1..32 (worker only)")
	speed := flag.Float64("speed", 1, "speech speed, 0.5..2 (worker only)")
	backend := flag.String("backend", tts.DefaultLocalBackend, "local inference backend: webgpu or cpu (worker only)")
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), `Usage: arcana-world-tts [options] <play|play-worker|worker>

play        Play MP3, PCM/float WAV, FLAC or Ogg Vorbis from stdin.
            Format is detected from the header; Opus/AAC are not supported.
            Mono/stereo; at most 16 MiB and 120 seconds.
            MP3: MPEG-1/2 Layer III, 16–48 kHz, CBR/VBR.
play-worker Serve audio frames using one resident device context.
            Initial stdout frame: "audio-play-v1"; completion reply: "played".
            Same audio limits as play; sample rates/formats may change.
            Device is suspended while idle; --volume applies to all requests.
            Exits on EOF or request failure; diagnostics use stderr.
worker      Load a local model once and serve bounded UTF-8 text frames.
            Frames: uint32 little-endian byte length followed by payload.
            Initial stdout frame: "local-tts-v1"; replies: mono PCM16 WAV.
            Melo: 44100 Hz; Kokoro: 24000 Hz; Piper: 22050 Hz.
            Text limit: 2048 bytes; audio limit: 120 seconds. EOF exits.
            Text is never accepted in command-line arguments.
            GPU (WebGPU) is the default; failures do not switch to CPU.
            Select --backend cpu if GPU initialization fails.
            WebGPU: macOS 14+ arm64; Linux/Windows amd64 or arm64.
            Linux requires a Vulkan loader and GPU driver.
            Keep the bundled native libraries and WebGPU plugin beside this
            helper; on Windows also keep matching dxcompiler.dll and dxil.dll.

Options must appear before the subcommand.`)
		fmt.Fprintln(flag.CommandLine.Output(), "Local TTS capability:", localCapability)
		flag.PrintDefaults()
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	var err error
	switch flag.Arg(0) {
	case "play":
		err = play(*volume)
	case "play-worker":
		err = playWorker(*volume)
	case "worker":
		err = runLocal(*modelDir, *threads, *speed, *backend, *provider, *speaker)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// withSignals 收到中断信号时取消上下文并关闭标准输入，让阻塞的读取立即返回。
func withSignals(run func(ctx context.Context) error) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			_ = os.Stdin.Close()
		case <-done:
		}
	}()
	return run(ctx)
}

func play(volume int) error {
	return withSignals(func(ctx context.Context) error { return audio.Run(ctx, os.Stdin, volume) })
}

func playWorker(volume int) error {
	return withSignals(func(ctx context.Context) error { return audio.RunWorker(ctx, os.Stdin, os.Stdout, volume) })
}
