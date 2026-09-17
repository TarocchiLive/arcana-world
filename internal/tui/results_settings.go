package tui

import (
	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
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
		m.log("TTS 停止失败：" + err.Error())
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
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	m.applyOverlaySettings(i18n.TUIOverlayRestoreDone)
	return m.updateOverlayChat()
}

var (
	obsPasswordOperation = completionOperation(i18n.TUIOperationUpdateOBSPassword)
	obsURLOperation      = completionOperation(i18n.TUIOperationUpdateOBSURL)
)
