package tui

import (
	"fmt"

	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
)

func settingsResetOperation() operation[*bili.Client] {
	return operation[*bili.Client]{label: i18n.TUISettingsReset, handle: (*Model).handleSettingsResetResult}
}

func (m *Model) handleSettingsResetResult(value *bili.Client, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	m.session.Release(m.client)
	m.client = value
	m.overlayEnabled = m.config.Overlay.Enabled
	if m.overlay != nil {
		m.overlay.options.Config = m.config.Overlay.Config("")
	}
	if err := m.closeTTS(); err != nil {
		m.warn(fmt.Sprintf(i18n.T(i18n.TTSLogStopFailed), err))
	}
	if m.ttsOverride != nil {
		m.tts.enabled = *m.ttsOverride
	}
	m.syncChat()
	m.log(i18n.T(i18n.TUISettingsResetDone))
	return tea.Batch(m.stopOverlay(), m.updateOverlayChat())
}

func configOperation() operation[*bili.Client] {
	return operation[*bili.Client]{label: i18n.TUIOperationSaveSettings, handle: (*Model).handleConfigResult}
}

func (m *Model) handleConfigResult(value *bili.Client, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	if value != nil {
		m.session.Release(m.client)
		m.client = value
	}
	m.log(i18n.T(i18n.TUILogSettingsSaved))
	m.syncChat()
	return m.finishResult()
}

func appearanceOperation() operation[struct{}] {
	return operation[struct{}]{label: i18n.TUIOperationSaveSettings, handle: (*Model).handleAppearanceResult}
}

func (m *Model) handleAppearanceResult(_ struct{}, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	// 仅刷新仍打开的菜单，不重新进入已被 Esc 关闭的选择器。
	if m.mode == "pick" && m.editKind == "appearance" {
		m.choices = m.appearanceChoices()
	}
	m.log(i18n.T(i18n.TUILogSettingsSaved))
	return m.finishResult()
}

func overlayConfigOperation() operation[struct{}] {
	return operation[struct{}]{label: i18n.TUIOperationSaveSettings, handle: (*Model).handleOverlayConfigResult}
}

func overlayToggleOperation() operation[struct{}] {
	return operation[struct{}]{label: i18n.TUIOperationSaveSettings, handle: (*Model).handleOverlayToggleResult}
}

func overlayRestoreOperation() operation[struct{}] {
	return operation[struct{}]{label: i18n.TUIOverlayRestore, handle: (*Model).handleOverlayRestoreResult}
}

func (m *Model) applyOverlaySettings(notice i18n.Key) {
	if m.overlay == nil {
		m.overlay = &overlayRuntime{state: "off"}
	}
	m.overlay.options.Config = m.config.Overlay.Config("")
	m.log(i18n.T(notice))
}

func (m *Model) handleOverlayConfigResult(_ struct{}, err error, label i18n.Key) tea.Cmd {
	defer func() {
		if m.overlaySettings != nil {
			m.showOverlaySettings()
		}
	}()
	if err != nil {
		m.clearOverlayColorPreview()
	} else {
		m.overlayColorPreview = nil
	}
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	m.applyOverlaySettings(i18n.TUILogSettingsSaved)
	return m.updateOverlayChat()
}

func (m *Model) handleOverlayToggleResult(_ struct{}, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	m.applyOverlaySettings(i18n.TUILogSettingsSaved)
	m.overlayEnabled = m.config.Overlay.Enabled
	var cmd tea.Cmd
	if m.overlayEnabled {
		cmd = m.startOverlay()
	} else {
		cmd = m.stopOverlay()
	}
	return tea.Batch(cmd, m.updateOverlayChat())
}

func (m *Model) handleOverlayRestoreResult(_ struct{}, err error, label i18n.Key) tea.Cmd {
	defer func() {
		if m.overlaySettings != nil {
			m.showOverlaySettings()
		}
	}()
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	m.applyOverlaySettings(i18n.TUIOverlayRestoreDone)
	return m.updateOverlayChat()
}

var (
	obsPasswordOperation = operation[struct{}]{label: i18n.TUIOperationUpdateOBSPassword, handle: (*Model).handleOBSPasswordResult}
	obsURLOperation      = completionOperation(i18n.TUIOperationUpdateOBSURL)
)

func (m *Model) handleOBSPasswordResult(_ struct{}, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	m.log(fmt.Sprintf(i18n.T(i18n.TUILogOperationSucceeded), i18n.T(label)))
	m.logCredentialStorage()
	return m.finishResult()
}
