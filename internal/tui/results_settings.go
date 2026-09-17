package tui

import (
	"arcana-world/internal/bili"
	"arcana-world/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
)

func settingsResetOperation() operation[*bili.Client] {
	return operation[*bili.Client]{name: "settings-reset", discardOnCancel: false, handle: (*Model).handleSettingsResetResult}
}

func (m *Model) handleSettingsResetResult(value *bili.Client, err error) tea.Cmd {

	if cmd, failed := m.resultError("settings-reset", err); failed {
		return cmd
	}
	m.session.Release(m.client)
	m.client = value
	m.overlayEnabled = m.config.Overlay.Enabled
	if m.overlay != nil {
		m.overlay.options.Config = m.config.Overlay.Config("")
	}
	m.syncChat()
	m.log(i18n.T(i18n.TUISettingsResetDone))
	return tea.Batch(m.stopOverlay(), m.updateOverlayChat())
}

func configOperation() operation[*bili.Client] {
	return operation[*bili.Client]{name: "config", discardOnCancel: false, handle: (*Model).handleConfigResult}
}

func (m *Model) handleConfigResult(value *bili.Client, err error) tea.Cmd {

	if cmd, failed := m.resultError("config", err); failed {
		return cmd
	}
	if c := value; c != nil {
		m.session.Release(m.client)
		m.client = c
	}
	m.log(i18n.T(i18n.TUILogSettingsSaved))
	m.syncChat()
	return m.finishResult()
}

func overlayConfigOperation() operation[struct{}] {
	return operation[struct{}]{name: "overlay-config", handle: func(m *Model, _ struct{}, err error) tea.Cmd {
		return m.handleOverlaySettingsResult("overlay-config", err)
	}}
}
func overlayToggleOperation() operation[struct{}] {
	return operation[struct{}]{name: "overlay-toggle", handle: func(m *Model, _ struct{}, err error) tea.Cmd {
		return m.handleOverlaySettingsResult("overlay-toggle", err)
	}}
}
func overlayRestoreOperation() operation[struct{}] {
	return operation[struct{}]{name: "overlay-restore", handle: func(m *Model, _ struct{}, err error) tea.Cmd {
		return m.handleOverlaySettingsResult("overlay-restore", err)
	}}
}
func (m *Model) handleOverlaySettingsResult(name string, err error) tea.Cmd {
	if cmd, failed := m.resultError(name, err); failed {
		return cmd
	}
	if m.overlay == nil {
		m.overlay = &overlayRuntime{state: "off"}
	}
	m.overlay.options.Config = m.config.Overlay.Config("")
	if name == "overlay-restore" {
		m.log(i18n.T(i18n.TUIOverlayRestoreDone))
	} else {
		m.log(i18n.T(i18n.TUILogSettingsSaved))
	}
	var cmd tea.Cmd
	if name == "overlay-toggle" {
		m.overlayEnabled = m.config.Overlay.Enabled
		if m.overlayEnabled {
			cmd = m.startOverlay()
		} else {
			cmd = m.stopOverlay()
		}
	}
	return tea.Batch(cmd, m.updateOverlayChat())
}

var (
	obsPasswordOperation = completionOperation("obs-password")
	obsURLOperation      = completionOperation("obs-url")
)
