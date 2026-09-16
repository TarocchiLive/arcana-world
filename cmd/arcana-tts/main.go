package main

import (
	"flag"
	"fmt"
	"os"

	"arcana-world/internal/tts"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: arcana-tts [-help]\nSynthesize one JSON request from stdin; write MP3 to stdout.")
	}
	flag.Parse()
	if flag.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "arcana-tts: unexpected arguments")
		os.Exit(2)
	}
	if err := tts.RunEdge(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
