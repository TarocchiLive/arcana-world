//go:build tts

package main

import (
	"os"

	"arcana-world/internal/tts/meloworker"
)

const localCapability = "sherpa-onnx v1.13.8, WebGPU (default) / CPU, Melo / Kokoro v1.1 / Piper"

func runLocal(dir string, threads int, speed float64, backend, provider string, speaker int) error {
	return meloworker.Run(os.Stdin, os.Stdout, dir, threads, speed, backend, provider, speaker)
}
