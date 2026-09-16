//go:build tts_audio

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"arcana-world/internal/tts/audio"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "Usage: arcana-audio [-help]\nPlay one 24000 Hz MP3 from stdin; stdout remains empty.")
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
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
	err := audio.Run(ctx, os.Stdin)
	close(done)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
