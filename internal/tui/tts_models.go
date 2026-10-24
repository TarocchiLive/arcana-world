package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	"arcana-world/internal/tts"
	"arcana-world/internal/tts/models"
	tea "charm.land/bubbletea/v2"
)

// 模型任务独立于账号，进度只保留最新值，不让下载等待界面。
type ttsModelJob struct {
	kind     string
	provider string
	cancel   context.CancelFunc
	done     chan struct{}
	mu       sync.Mutex
	progress models.Progress
	err      error
}

// 只在用户主动选择且保存成功后提示；本次运行取消后不重复打扰。
func (m *Model) selectTTSProvider(provider string) tea.Cmd {
	cfg := m.config
	cfg.TTS.Provider = provider
	op := configOperation()
	saved := op.handle
	op.handle = func(m *Model, client *bili.Client, err error, label i18n.Key) tea.Cmd {
		cmd := saved(m, client, err, label)
		_, local := models.SpecFor(provider)
		if err != nil || m.canceled || !local || m.config.TTS.Provider != provider {
			return cmd
		}
		m.syncTTS()
		r := m.tts
		if r.modelPromptShown[provider] || r.modelJob != nil {
			return cmd
		}
		r.modelStatus = models.Inspect(m.appDir, provider)
		if !r.modelStatus.Installed {
			if r.modelPromptShown == nil {
				r.modelPromptShown = make(map[string]bool)
			}
			r.modelPromptShown[provider] = true
			return tea.Batch(cmd, m.confirmTTSModelInstall(true))
		}
		return cmd
	}
	return work(m, op, func(ctx context.Context) (*bili.Client, error) {
		if err := m.session.Lock(ctx); err != nil {
			return nil, err
		}
		defer m.session.Unlock()
		return nil, m.store.SaveConfig(cfg)
	})
}

// 确认前只检查本地压缩包，不创建目录、不启动下载或模型任务。
func (m *Model) confirmTTSModelInstall(firstChoice bool) tea.Cmd {
	provider := m.config.TTS.Provider
	spec, ok := models.SpecFor(provider)
	if !ok {
		return nil
	}
	archive := models.ArchivePath(m.appDir, provider)
	sizeMB := (spec.ArchiveSize + 500000) / 1000000
	prompt := fmt.Sprintf(i18n.T(i18n.TTSModelInstallDownloadConfirm), filepath.Base(archive), sizeMB)
	if _, err := os.Lstat(archive); err == nil {
		prompt = fmt.Sprintf(i18n.T(i18n.TTSModelInstallLocalConfirm), filepath.Base(archive))
	} else if !errors.Is(err, os.ErrNotExist) {
		m.warn(m.safe(err.Error()))
		return nil
	} else if firstChoice {
		info, _ := tts.ProviderByID(provider)
		prompt = fmt.Sprintf(i18n.T(i18n.TTSModelDownloadConfirm), sizeMB, i18n.T(info.SettingsTitle))
	}
	m.tts.modelConfirmProvider = provider
	return m.confirm(prompt, "tts-model-install-confirmed")
}

func (m *Model) performTTSModel(action string) tea.Cmd {
	r := m.tts
	if action == "tts-model-cancel" {
		if r.modelJob != nil {
			r.modelJob.cancel()
		}
		return nil
	}
	provider, ok := tts.ProviderByID(m.config.TTS.Provider)
	if !ok || !provider.Model || r.modelJob != nil {
		return nil
	}
	if action == "tts-model-install" {
		return m.confirmTTSModelInstall(false)
	}
	if action == "tts-model-install-confirmed" {
		confirmed := r.modelConfirmProvider
		r.modelConfirmProvider = ""
		if confirmed != provider.ID {
			return nil
		}
	}
	m.syncTTS()
	r.retire()
	r.preview = false
	ctx, cancel := context.WithCancel(m.ctx)
	stage := "installing"
	if action == "tts-model-verify" {
		stage = "verifying"
		m.log(i18n.T(i18n.TTSModelVerifying))
	}
	job := &ttsModelJob{kind: action, provider: provider.ID, cancel: cancel, done: make(chan struct{}), progress: models.Progress{Stage: stage}}
	r.modelJob = job
	retired := append([]*ttsRetired(nil), r.retired...)
	appDir, proxy := m.appDir, m.config.Proxy
	// 先回收使用旧模型的进程，再校验或发布新模型。
	go func() {
		defer close(job.done)
		defer cancel()
		for _, old := range retired {
			select {
			case <-ctx.Done():
				job.err = ctx.Err()
				return
			case <-old.done:
				// 旧进程的关闭错误由 syncTTS 展示，不应让模型任务失败。
			}
		}
		if action == "tts-model-verify" {
			job.err = models.Verify(ctx, models.DefaultDir(appDir, job.provider), job.provider)
		} else {
			job.err = models.Install(ctx, appDir, proxy, job.provider, func(p models.Progress) {
				job.mu.Lock()
				job.progress = p
				job.mu.Unlock()
			})
		}
	}()
	return nil
}

func (m *Model) pollTTSModel() {
	r := m.tts
	if r == nil || r.modelJob == nil {
		return
	}
	job := r.modelJob
	select {
	case <-job.done:
		r.modelJob = nil
		r.modelStatus = models.Inspect(m.appDir, m.config.TTS.Provider)
		r.failed = false
		switch {
		case errors.Is(job.err, context.Canceled):
			m.log(i18n.T(i18n.TTSModelCanceled))
		case job.err != nil:
			m.warn(fmt.Sprintf(i18n.T(i18n.TTSModelFailed), m.safe(job.err.Error())))
		default:
			message := i18n.TTSModelDone
			if job.kind == "tts-model-verify" {
				message = i18n.TTSModelVerifyDone
			}
			m.log(i18n.T(message))
		}
	default:
	}
	m.refreshTTSModelChoices()
}

func (m *Model) ttsModelText() string {
	// 渲染只读缓存；供应商变化与模型任务完成时刷新文件状态。
	status := models.Status{Dir: models.DefaultDir(m.appDir, m.config.TTS.Provider), Archive: models.ArchivePath(m.appDir, m.config.TTS.Provider)}
	if m.tts != nil && m.tts.modelStatus.Dir == status.Dir {
		status = m.tts.modelStatus
	}
	state := i18n.T(i18n.TTSModelMissing)
	if status.Installed {
		state = i18n.T(i18n.TTSModelInstalled)
	}
	if m.tts != nil {
		if job := m.tts.modelJob; job != nil {
			job.mu.Lock()
			p := job.progress
			job.mu.Unlock()
			stage := i18n.T(i18n.TTSModelBusy)
			switch p.Stage {
			case "downloading":
				stage = i18n.T(i18n.TTSModelDownloading)
			case "verifying":
				stage = i18n.T(i18n.TTSModelVerifying)
			case "extracting":
				stage = i18n.T(i18n.TTSModelExtracting)
			case "ready":
				stage = i18n.T(i18n.TTSModelReady)
			}
			state = stage
			if p.Stage == "downloading" {
				const mb = 1 << 20
				downloaded := float64(p.Downloaded) / mb
				if p.Total > 0 {
					state = fmt.Sprintf(i18n.T(i18n.TTSModelWorking), stage, downloaded, float64(p.Total)/mb, min(100, p.Downloaded*100/p.Total))
				} else {
					state = fmt.Sprintf(i18n.T(i18n.TTSModelWorkingUnknown), stage, downloaded)
				}
			}
		}
	}
	return fmt.Sprintf(i18n.T(i18n.TTSModelStatus), state, status.Dir, status.Archive)
}

// 模型动作在终端与网页共用，取消按钮只描述当前阶段。
func (m *Model) ttsModelActions() []choice {
	if m.tts != nil && m.tts.modelJob != nil {
		job := m.tts.modelJob
		job.mu.Lock()
		stage := job.progress.Stage
		job.mu.Unlock()
		label := i18n.TTSModelCancelInstall
		switch stage {
		case "downloading":
			label = i18n.TTSModelCancelDownload
		case "verifying":
			label = i18n.TTSModelCancelVerify
		}
		return []choice{{i18n.T(label), "tts-model-cancel"}}
	}
	return []choice{
		{i18n.T(i18n.TTSModelInstall), "tts-model-install"},
		{i18n.T(i18n.TTSModelVerify), "tts-model-verify"},
	}
}

// 保留设置字段和光标位置，只替换任务变化影响的模型动作。
func (m *Model) refreshTTSModelChoices() {
	if m.mode != "pick" || m.editKind != "tts-settings" || !m.ttsProvider().Model {
		return
	}
	offset := len(m.ttsProvider().Fields)
	actions := m.ttsModelActions()
	unchanged := len(m.choices) == offset+len(actions)
	if unchanged {
		for i, action := range actions {
			if m.choices[offset+i] != action {
				unchanged = false
				break
			}
		}
	}
	if unchanged {
		return
	}
	m.choices = append(m.choices[:offset], actions...)
	if m.selected >= len(m.choices) {
		m.selected = len(m.choices) - 1
	}
}
