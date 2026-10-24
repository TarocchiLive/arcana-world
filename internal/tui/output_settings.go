package tui

import (
	"errors"
	"slices"

	"arcana-world/internal/i18n"
	"arcana-world/internal/presentation"
	"arcana-world/internal/tts"
	tea "charm.land/bubbletea/v2"
)

func (m *Model) outputMenu() []menuItem {
	if m.page == overlayPage {
		return []menuItem{
			{toggleLabel(i18n.T(i18n.TUIOverlayEnabled), m.overlayEnabled), "overlay-toggle"},
			{i18n.T(i18n.TUIOverlayBackend) + ": " + overlayBackendLabel(m.config.Overlay.Backend), "overlay-backend"},
			{i18n.T(i18n.OutputDisplays), "overlay-displays"},
			{i18n.T(i18n.TUIOverlaySettings), "overlay-settings"},
			{toggleLabel(i18n.T(i18n.TUIOverlayPreview), m.overlay != nil && m.overlay.preview), "overlay-preview"},
			{i18n.T(i18n.OutputEvents), "output-events"},
		}
	}
	return []menuItem{
		{i18n.T(i18n.OutputTTSToggle), "tts-toggle"},
		{i18n.T(i18n.TTSProvider), "tts-provider"},
		{i18n.T(m.ttsProvider().SettingsTitle), "tts-settings"},
		{i18n.T(i18n.OutputVolume), "tts-volume"},
		{i18n.T(i18n.OutputTTSStop), "tts-stop"},
		{i18n.T(i18n.OutputTTSPreview), "tts-preview"},
		{i18n.T(i18n.OutputEvents), "output-events"},
	}
}

func (m *Model) pickOutputEvents() tea.Cmd {
	m.choices = nil
	for _, spec := range presentation.Specs() {
		m.choices = append(m.choices, choice{i18n.T(spec.Label), spec.ID})
	}
	title := i18n.OutputOverlayEvents
	if m.page == ttsPage {
		title = i18n.OutputTTSEvents
	}
	return m.pick("output-events", i18n.T(title))
}

func (m *Model) outputEventLabel(ch choice) string {
	disabled := m.config.OverlayDisabledEvents
	if m.page == ttsPage {
		disabled = m.config.TTS.DisabledEvents
	}
	mark := "[x] "
	if slices.Contains(disabled, ch.value) {
		mark = "[ ] "
	}
	return mark + ch.label
}

func (m *Model) toggleOutputEvent(id string) tea.Cmd {
	if m.page != overlayPage && m.page != ttsPage {
		return nil
	}
	known := false
	for _, spec := range presentation.Specs() {
		if spec.ID == id {
			known = true
			break
		}
	}
	if !known {
		return nil
	}
	cfg := m.config
	disabled := cfg.OverlayDisabledEvents
	if m.page == ttsPage {
		disabled = cfg.TTS.DisabledEvents
	}
	disabled = slices.Clone(disabled)
	if slices.Contains(disabled, id) {
		disabled = slices.DeleteFunc(disabled, func(value string) bool { return value == id })
	} else {
		disabled = append(disabled, id)
	}
	if m.page == ttsPage {
		cfg.TTS.DisabledEvents = disabled
	} else {
		cfg.OverlayDisabledEvents = disabled
	}
	return m.saveConfig(cfg, false)
}

func (m *Model) ttsProvider() tts.Provider {
	provider, _ := tts.ProviderByID(m.config.TTS.Provider)
	return provider
}

func (m *Model) pickTTSProvider() tea.Cmd {
	m.choices = nil
	selected := 0
	for _, provider := range tts.Providers() {
		if provider.ID == m.config.TTS.Provider {
			selected = len(m.choices)
		}
		m.choices = append(m.choices, choice{i18n.T(provider.Name), provider.ID})
	}
	cmd := m.pick("tts-provider", i18n.T(i18n.TTSProvider))
	m.selected = selected
	return cmd
}

func (m *Model) pickTTSSettings() tea.Cmd {
	provider := m.ttsProvider()
	values := m.config.TTS.Values()
	m.choices = nil
	for _, field := range provider.Fields {
		if field.Secret {
			m.choices = append(m.choices, choice{i18n.T(field.Label), "tts-field:" + field.ID})
			continue
		}
		value := values[field.ID]
		for _, option := range field.Options {
			if option.ID == value {
				value = i18n.T(option.Label)
				break
			}
		}
		m.choices = append(m.choices, choice{i18n.T(field.Label) + ": " + value, "tts-field:" + field.ID})
	}
	if provider.Model {
		m.choices = append(m.choices, m.ttsModelActions()...)
	}
	return m.pick("tts-settings", i18n.T(provider.SettingsTitle))
}

func (m *Model) editTTSField(id string) tea.Cmd {
	for _, field := range m.ttsProvider().Fields {
		if field.ID != id {
			continue
		}
		value := m.config.TTS.Values()[id]
		if len(field.Options) == 0 {
			if field.Secret {
				value = ""
			}
			return m.form("tts-field:"+id, i18n.T(field.Label), value, field.Secret)
		}
		m.choices = nil
		selected := 0
		for _, option := range field.Options {
			if option.ID == value {
				selected = len(m.choices)
			}
			m.choices = append(m.choices, choice{i18n.T(option.Label), option.ID})
		}
		cmd := m.pick("tts-field:"+id, i18n.T(field.Label))
		m.selected = selected
		return cmd
	}
	return nil
}

func (m *Model) saveTTSField(id, value string) tea.Cmd {
	cmd, err := m.setTTSField(id, value)
	if err != nil {
		m.warnStatus(clean(err.Error()))
		return cmd
	}
	// 只有终端自己的编辑框保存成功后才关闭；网页提交不影响终端当前界面。
	m.mode = ""
	m.input.SetValue("")
	m.input.Blur()
	return cmd
}

func (m *Model) setTTSField(id, value string) (tea.Cmd, error) {
	secret := false
	known := false
	for _, field := range m.ttsProvider().Fields {
		if field.ID == id {
			known, secret = true, field.Secret
			break
		}
	}
	if !known {
		return nil, errors.New(i18n.T(i18n.WebRequestInvalid))
	}
	cfg := m.config
	settings, err := cfg.TTS.WithValue(id, value).Normalize()
	if err != nil {
		return nil, err
	}
	if secret {
		return m.saveTTSSecret(id, settings.Values()[id]), nil
	}
	cfg.TTS = settings
	return m.saveConfig(cfg, false), nil
}
