package tts

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync/atomic"
	"time"

	"arcana-world/internal/i18n"
	"arcana-world/internal/tts/meloproto"
	"arcana-world/internal/tts/models"
)

const DefaultLocalThreads = 4
const DefaultLocalBackend = "webgpu"

type LocalOptions struct {
	Provider   string
	Speaker    int
	Executable string
	ModelDir   string
	Threads    int
	Backend    string
	Speed      float64
	Timeout    time.Duration
}

// Local 独占一个常驻 worker，取消与 Close 均等待进程回收。
type Local struct {
	options LocalOptions
	client  *residentClient
	// 模型文件只在首次合成前完整校验；worker 因取消或超时重启时无需重复散列。
	verified atomic.Bool
}

func NewLocal(options LocalOptions) (Synthesizer, error) {
	spec, ok := models.SpecFor(options.Provider)
	if !ok {
		return nil, errors.New("local TTS: unknown provider")
	}
	if options.Speaker < 0 || options.Speaker >= spec.Speakers {
		return nil, fmt.Errorf("local TTS: speaker must be 0..%d", spec.Speakers-1)
	}
	if options.Backend == "" {
		options.Backend = DefaultLocalBackend
	}
	if options.Backend != "webgpu" && options.Backend != "cpu" {
		return nil, errors.New("local TTS: backend must be webgpu or cpu")
	}
	if options.Threads == 0 {
		options.Threads = DefaultLocalThreads
	}
	if options.Speed == 0 {
		options.Speed = 1
	}
	if options.Timeout == 0 {
		options.Timeout = 120 * time.Second
	}
	if options.Threads < 1 || options.Threads > 32 || math.IsNaN(options.Speed) || math.IsInf(options.Speed, 0) || options.Speed < 0.5 || options.Speed > 2 || options.Timeout < time.Second || options.Timeout > 10*time.Minute {
		return nil, errors.New("local TTS: threads must be 1..32, speed 0.5..2 and timeout 1s..10m")
	}
	if options.ModelDir == "" {
		return nil, errors.New("local TTS: model directory is required")
	}
	dir, err := filepath.Abs(options.ModelDir)
	if err != nil {
		return nil, err
	}
	options.ModelDir = dir
	executable, err := resolveHelper("arcana-world-tts", options.Executable)
	if err != nil {
		logger.Error("local synthesizer setup failed", "provider", options.Provider, "model_dir", options.ModelDir, "backend", options.Backend, "executable", options.Executable, "stage", "resolve_helper", "err", err)
		return nil, err
	}
	options.Executable = executable
	return &Local{options: options, client: newResidentClient()}, nil
}

func (m *Local) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("local TTS: nil context")
	}
	if !meloproto.ValidText(text) {
		return nil, errors.New("local TTS: text must contain 1..2048 valid UTF-8 bytes without NUL")
	}
	started := time.Now()
	ctx, cancel := context.WithTimeout(ctx, m.options.Timeout)
	defer cancel()
	if !m.verified.Load() {
		// 缺失或损坏的模型直接返回本地化的模型错误，不当作 worker 故障。
		if err := models.Verify(ctx, m.options.ModelDir, m.options.Provider); err != nil {
			if !errors.Is(err, context.Canceled) {
				logger.Warn("local synthesis failed", "provider", m.options.Provider, "model_dir", m.options.ModelDir, "backend", m.options.Backend, "executable", m.options.Executable, "stage", "verify_model", "elapsed", time.Since(started), "err", err)
			}
			return nil, err
		}
		m.verified.Store(true)
	}
	spec, _ := models.SpecFor(m.options.Provider)
	audio, err := m.client.request(ctx, func(context.Context) (*exec.Cmd, error) {
		logger.Info("local worker starting", "provider", m.options.Provider, "model_dir", m.options.ModelDir, "backend", m.options.Backend, "executable", m.options.Executable, "threads", m.options.Threads)
		return exec.Command(m.options.Executable, "--provider", m.options.Provider, "--speaker", strconv.Itoa(m.options.Speaker), "--model-dir", m.options.ModelDir, "--threads", strconv.Itoa(m.options.Threads), "--speed", strconv.FormatFloat(m.options.Speed, 'g', -1, 64), "--backend", m.options.Backend, "worker"), nil
	}, meloproto.Ready, []byte(text), 44+2*meloproto.SampleLimit(spec.SampleRate), func(data []byte) bool {
		return meloproto.ValidWAV(data, spec.SampleRate)
	})
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, ErrClosed) {
		return audio, err
	}
	logger.Warn("local synthesis failed", "provider", m.options.Provider, "model_dir", m.options.ModelDir, "backend", m.options.Backend, "executable", m.options.Executable, "stage", "worker_request", "elapsed", time.Since(started), "err", err)
	// 显卡驱动或 WebGPU 运行库故障重试无法恢复；超时由播报队列统一转换。
	if m.options.Backend == "webgpu" && !errors.Is(err, context.DeadlineExceeded) {
		return nil, errors.New(i18n.T(i18n.TTSLocalGPUFailed))
	}
	return nil, err
}

func (m *Local) Close() error {
	return m.client.Close()
}
