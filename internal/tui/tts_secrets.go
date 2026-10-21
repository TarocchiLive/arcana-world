package tui

import (
	"context"
	"fmt"

	"arcana-world/internal/i18n"
	"arcana-world/internal/tts"
	tea "charm.land/bubbletea/v2"
)

type ttsSecretsJob struct {
	done   chan struct{}
	values map[string]string
	err    error
}

// 密钥环可能等待系统授权，读取放在后台，界面只轮询已完成的结果。
func (m *Model) newTTSSynthesizer() (tts.Synthesizer, bool, error) {
	provider := m.ttsProvider()
	settings := m.config.TTS
	needsSecrets := false
	for _, field := range provider.Fields {
		needsSecrets = needsSecrets || field.Secret
	}
	if needsSecrets {
		r := m.tts
		if r.secrets == nil {
			job := &ttsSecretsJob{done: make(chan struct{})}
			r.secrets = job
			ctx, session, store := m.ctx, m.session, m.store
			go func() {
				defer close(job.done)
				if job.err = session.Lock(ctx); job.err != nil {
					return
				}
				defer session.Unlock()
				job.values, job.err = store.TTSSecrets(provider.ID)
			}()
		}
		select {
		case <-r.secrets.done:
			if err := r.secrets.err; err != nil {
				// 不缓存失败结果，用户授权钥匙串后重试即可恢复。
				r.secrets = nil
				return nil, true, err
			}
			for _, field := range provider.Fields {
				if field.Secret {
					settings = settings.WithValue(field.ID, r.secrets.values[field.ID])
				}
			}
		default:
			return nil, false, nil
		}
	}
	synthesizer, err := tts.NewSynthesizer(settings, m.appDir, m.tts.proxy)
	return synthesizer, true, err
}

var ttsSecretOperation = operation[struct{}]{label: i18n.TTSSecretSave, handle: (*Model).handleTTSSecretResult}

func (m *Model) saveTTSSecret(field, value string) tea.Cmd {
	m.syncTTS()
	provider := m.config.TTS.Provider
	cmd := work(m, ttsSecretOperation, func(ctx context.Context) (struct{}, error) {
		if err := m.session.Lock(ctx); err != nil {
			return struct{}{}, err
		}
		defer m.session.Unlock()
		return struct{}{}, m.store.SetTTSSecret(provider, field, value)
	})
	if cmd != nil {
		m.tts.retire()
		m.tts.identity = ""
		m.tts.preview = false
		m.tts.secretSaving = true
	}
	return cmd
}

func (m *Model) handleTTSSecretResult(_ struct{}, err error, label i18n.Key) tea.Cmd {
	m.tts.secrets = nil
	m.tts.secretSaving, m.tts.failed = false, false
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	m.log(fmt.Sprintf(i18n.T(i18n.TUILogOperationSucceeded), i18n.T(label)))
	m.logCredentialStorage()
	return m.finishResult()
}
