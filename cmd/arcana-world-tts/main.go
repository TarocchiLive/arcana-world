//go:build tts_audio

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
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: arcana-world-tts [--volume 0..100] <synthesize|play>\nSynthesize one JSON request to MP3, or play one 24000 Hz MP3.")
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
		os.Exit(2)
	}

	var err error
	switch flag.Arg(0) {
	case "synthesize":
		err = tts.RunEdge(os.Stdin, os.Stdout)
	case "play":
		err = play(*volume)
	default:
		flag.Usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func play(volume int) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = os.Stdin.Close()
		case <-done:
		}
	}()
	err := audio.Run(ctx, os.Stdin, volume)
	close(done)
	return err
}
