//go:build tts && cgo

// Package meloworker 只由统一 TTS 辅助程序引用，必须链接原生运行库。
package meloworker

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"arcana-world/internal/tts/meloproto"
	"arcana-world/internal/tts/models"
	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"
)

func Run(input io.Reader, output io.Writer, dir string, threads int, speed float64, backend, provider string, speaker int) error {
	spec, ok := models.SpecFor(provider)
	if !ok || speaker < 0 || speaker >= spec.Speakers {
		return errors.New("local TTS: invalid provider or speaker")
	}
	if backend == "" {
		backend = "webgpu"
	}
	if backend != "cpu" && backend != "webgpu" {
		return errors.New("local TTS: backend must be webgpu or cpu")
	}
	if backend == "webgpu" {
		if sherpa.GetVersion() != "1.13.8-arcana-webgpu2" {
			return errors.New("local TTS: bundled native runtime lacks WebGPU support; rebuild with bash scripts/build.sh tts or select cpu")
		}
		executable, err := os.Executable()
		if err != nil {
			return fmt.Errorf("local TTS: locate WebGPU helper: %w", err)
		}
		executable, err = filepath.EvalSymlinks(executable)
		if err != nil {
			return fmt.Errorf("local TTS: resolve WebGPU helper: %w", err)
		}
		library := "libonnxruntime_providers_webgpu.so"
		switch runtime.GOOS {
		case "darwin":
			library = "libonnxruntime_providers_webgpu.dylib"
		case "windows":
			library = "onnxruntime_providers_webgpu.dll"
		}
		library = filepath.Join(filepath.Dir(executable), library)
		if info, err := os.Stat(library); err != nil || !info.Mode().IsRegular() {
			return errors.New("local TTS: bundled WebGPU plugin is missing beside the helper; rebuild with bash scripts/build.sh tts or select cpu")
		}
		// 独立 worker 只有一个模型；路径只传给本进程的原生模型会话。
		if err := os.Setenv("ARCANA_LOCAL_WEBGPU_LIBRARY", library); err != nil {
			return fmt.Errorf("local TTS: configure WebGPU plugin: %w", err)
		}
		if err := configureVulkanDrivers(); err != nil {
			return fmt.Errorf("local TTS: configure Vulkan driver search: %w", err)
		}
	}
	if dir == "" || threads < 1 || threads > 32 || math.IsNaN(speed) || math.IsInf(speed, 0) || speed < 0.5 || speed > 2 {
		return errors.New("local TTS: invalid worker configuration")
	}
	// FST 与词典列表使用逗号分隔；不能把含逗号的目录拆成多个路径。
	if strings.Contains(dir, ",") {
		return errors.New("local TTS: model directory cannot contain a comma")
	}
	path := func(name string) string { return filepath.Join(dir, name) }
	config := sherpa.OfflineTtsConfig{
		Model:           sherpa.OfflineTtsModelConfig{NumThreads: threads, Provider: backend},
		MaxNumSentences: 1, SilenceScale: 1,
	}
	switch provider {
	case "melo":
		config.Model.Vits = sherpa.OfflineTtsVitsModelConfig{Model: path("model.onnx"), Tokens: path("tokens.txt"), Lexicon: path("lexicon.txt"), DictDir: path("dict"), NoiseScale: 0.667, NoiseScaleW: 0.8, LengthScale: 1}
		config.RuleFsts = strings.Join([]string{path("date.fst"), path("number.fst"), path("phone.fst"), path("new_heteronym.fst")}, ",")
	case "kokoro":
		config.Model.Kokoro = sherpa.OfflineTtsKokoroModelConfig{
			Model: path("model.onnx"), Voices: path("voices.bin"), Tokens: path("tokens.txt"),
			DataDir: path("espeak-ng-data"), Lexicon: strings.Join([]string{path("lexicon-us-en.txt"), path("lexicon-zh.txt")}, ","), LengthScale: 1,
		}
	case "piper":
		config.Model.Vits = sherpa.OfflineTtsVitsModelConfig{Model: path("en_US-ljspeech-medium.onnx"), Tokens: path("tokens.txt"), DataDir: path("espeak-ng-data"), NoiseScale: 0.667, NoiseScaleW: 0.8, LengthScale: 1}
	}
	model := sherpa.NewOfflineTts(&config)
	if model == nil {
		return errors.New("local TTS: native model initialization failed")
	}
	defer sherpa.DeleteOfflineTts(model)
	if model.SampleRate() != spec.SampleRate || model.NumSpeakers() != spec.Speakers {
		return fmt.Errorf("local TTS: expected %d Hz and %d speakers", spec.SampleRate, spec.Speakers)
	}
	maxSamples := meloproto.SampleLimit(spec.SampleRate)
	if maxSamples == 0 {
		return errors.New("local TTS: unsupported sample rate")
	}
	if err := meloproto.Write(output, []byte(meloproto.Ready)); err != nil {
		return err
	}
	generation := &sherpa.GenerationConfig{Sid: speaker, Speed: float32(speed), SilenceScale: config.SilenceScale}
	for {
		text, err := meloproto.Read(input, meloproto.MaxText)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if !meloproto.ValidText(string(text)) {
			return errors.New("local TTS: invalid synthesis input")
		}
		// 单说话人 VITS 的 sid=0 由原生库映射到模型 metadata speaker_id。
		samples := 0
		oversized := false
		audio := model.GenerateWithConfig(string(text), generation, func(chunk []float32, _ float32) bool {
			if len(chunk) > maxSamples-samples {
				oversized = true
				return false
			}
			samples += len(chunk)
			return true
		})
		if oversized {
			return errors.New("local TTS: audio exceeds 120 seconds")
		}
		if audio == nil || audio.SampleRate != spec.SampleRate || len(audio.Samples) == 0 {
			return errors.New("local TTS: invalid native audio")
		}
		if len(audio.Samples) > maxSamples {
			return errors.New("local TTS: audio exceeds 120 seconds")
		}
		wav := make([]byte, 44+2*len(audio.Samples))
		copy(wav, "RIFF")
		binary.LittleEndian.PutUint32(wav[4:], uint32(len(wav)-8))
		copy(wav[8:], "WAVEfmt ")
		binary.LittleEndian.PutUint32(wav[16:], 16)
		binary.LittleEndian.PutUint16(wav[20:], 1)
		binary.LittleEndian.PutUint16(wav[22:], 1)
		binary.LittleEndian.PutUint32(wav[24:], uint32(spec.SampleRate))
		binary.LittleEndian.PutUint32(wav[28:], uint32(spec.SampleRate*2))
		binary.LittleEndian.PutUint16(wav[32:], 2)
		binary.LittleEndian.PutUint16(wav[34:], 16)
		copy(wav[36:], "data")
		binary.LittleEndian.PutUint32(wav[40:], uint32(len(wav)-44))
		for i, sample := range audio.Samples {
			if math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
				return errors.New("local TTS: non-finite PCM sample")
			}
			value := math.Round(float64(sample) * 32768)
			value = min(32767, max(-32768, value))
			binary.LittleEndian.PutUint16(wav[44+i*2:], uint16(int16(value)))
		}
		if err := meloproto.Write(output, wav); err != nil {
			return err
		}
	}
}

// 便携包的 Vulkan loader 不含 NixOS 的编译期搜索路径；只追加系统驱动目录，
// 保留标准发现、多显卡枚举及用户显式指定的驱动，不复制或解析厂商清单。
func configureVulkanDrivers() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	for _, key := range []string{"VK_DRIVER_FILES", "VK_ICD_FILENAMES"} {
		if _, ok := os.LookupEnv(key); ok {
			return nil
		}
	}
	dir := "/run/opengl-driver/share/vulkan/icd.d"
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return nil
	}
	existing := os.Getenv("VK_ADD_DRIVER_FILES")
	for _, entry := range filepath.SplitList(existing) {
		if filepath.Clean(entry) == dir {
			return nil
		}
	}
	if existing != "" {
		dir = existing + string(os.PathListSeparator) + dir
	}
	return os.Setenv("VK_ADD_DRIVER_FILES", dir)
}
