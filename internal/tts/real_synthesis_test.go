//go:build ttssmoke

package tts_test

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"arcana-world/internal/helperpath"
	"arcana-world/internal/journal"
	"arcana-world/internal/tts"
	"arcana-world/internal/tts/models"
	"github.com/hajimehoshi/go-mp3"
)

// 有效语音至少 0.3 秒，且峰值高于约 -30 dBFS，排除空白或静音输出。
const (
	minSpeechSeconds = 0.3
	minSpeechPeak    = 1000
)

// TestRealSynthesis 走真实链路：Edge 在线服务，或安装固定模型后由已部署的语音 helper 合成。
// 测试二进制须放在应用目录（与 libexec/ 同级），模型安装在该目录的 models/tts/。
// ARCANA_TTS_SMOKE 指定逗号分隔的供应商；ARCANA_TTS_SMOKE_BACKEND 指定本地运行设备。
func TestRealSynthesis(t *testing.T) {
	providers := strings.FieldsFunc(os.Getenv("ARCANA_TTS_SMOKE"), func(r rune) bool { return r == ',' || r == ' ' })
	if len(providers) == 0 {
		t.Fatal("ARCANA_TTS_SMOKE must list providers, e.g. edge,melo,kokoro,piper")
	}
	appDir, err := helperpath.ApplicationDir()
	if err != nil {
		t.Fatal(err)
	}
	backend := os.Getenv("ARCANA_TTS_SMOKE_BACKEND")
	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			logPath := openJournal(t)
			settings := tts.Settings{Provider: provider, Volume: 80}
			text := "欢迎来到直播间，感谢大家的支持。"
			if provider == "piper" {
				text = "Thanks for watching the stream."
			}
			if _, ok := models.SpecFor(provider); ok {
				if backend != "" {
					settings = settings.WithValue("backend", backend)
				}
				installModel(t, appDir, provider)
			}
			synthesizer, err := tts.NewSynthesizer(settings, appDir, "")
			if err != nil {
				t.Fatalf("create %s synthesizer: %v", provider, err)
			}
			// Close 须回收 helper 进程并结束进行中的请求；失败视为资源泄漏。
			t.Cleanup(func() {
				if err := synthesizer.Close(); err != nil {
					t.Errorf("close %s synthesizer: %v", provider, err)
				}
			})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			started := time.Now()
			audio, err := synthesizer.Synthesize(ctx, text)
			if err != nil {
				dumpJournal(t, logPath)
				t.Fatalf("%s synthesis failed after %s: %v", provider, time.Since(started).Round(time.Millisecond), err)
			}
			seconds, peak, err := measure(audio)
			if err != nil {
				t.Fatalf("%s returned invalid audio (%d bytes): %v", provider, len(audio), err)
			}
			t.Logf("%s: %d bytes, %.2fs, peak %d, %s", provider, len(audio), seconds, peak, time.Since(started).Round(time.Millisecond))
			if seconds < minSpeechSeconds || peak < minSpeechPeak {
				t.Fatalf("%s audio is empty or silent: %.2fs, peak %d", provider, seconds, peak)
			}
		})
	}
}

// openJournal 把 tts 调试日志写入临时目录，失败时输出其中的阶段与错误类别。
func openJournal(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log, err := journal.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	journal.SetLevel(slog.LevelDebug)
	t.Cleanup(func() {
		if err := log.Close(); err != nil {
			t.Errorf("close journal: %v", err)
		}
	})
	return filepath.Join(dir, "logs", "arcana-world.log")
}

func dumpJournal(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Logf("read journal: %v", err)
		return
	}
	t.Logf("tts journal:\n%s", data)
}

// installModel 复用应用的下载、散列校验与解压流程；缓存的压缩包会被直接校验安装。
func installModel(t *testing.T, appDir, provider string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	started := time.Now()
	stage := ""
	err := models.Install(ctx, appDir, "", provider, func(p models.Progress) {
		if p.Stage != stage {
			stage = p.Stage
			t.Logf("%s model: %s (%d/%d bytes)", provider, stage, p.Downloaded, p.Total)
		}
	})
	if err != nil {
		t.Fatalf("install %s model at stage %q after %s: %v", provider, stage, time.Since(started).Round(time.Second), err)
	}
}

// measure 解码 WAV（本地模型）或 MP3（Edge），返回时长与 16 位样本峰值。
func measure(audio []byte) (float64, int, error) {
	if len(audio) >= 12 && string(audio[:4]) == "RIFF" && string(audio[8:12]) == "WAVE" {
		// 本地 helper 固定输出 44 字节头的单声道 PCM16，格式已由合成器校验。
		if len(audio) < 46 {
			return 0, 0, errors.New("WAV without samples")
		}
		rate := binary.LittleEndian.Uint32(audio[24:28])
		pcm := audio[44:]
		return float64(len(pcm)/2) / float64(rate), peak16(pcm), nil
	}
	decoder, err := mp3.NewDecoder(bytes.NewReader(audio))
	if err != nil {
		return 0, 0, err
	}
	pcm, err := io.ReadAll(decoder)
	if err != nil {
		return 0, 0, err
	}
	// go-mp3 输出双声道 PCM16。
	return float64(len(pcm)/4) / float64(decoder.SampleRate()), peak16(pcm), nil
}

func peak16(pcm []byte) int {
	peak := 0
	for i := 0; i+1 < len(pcm); i += 2 {
		v := int(int16(binary.LittleEndian.Uint16(pcm[i:])))
		if v < 0 {
			v = -v
		}
		peak = max(peak, v)
	}
	return peak
}
