package tui

import (
	"slices"

	"arcana-world/internal/i18n"
	"arcana-world/internal/presentation"
	"arcana-world/internal/tts"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) outputMenu() []menuItem {
	if m.page == overlayPage {
		return []menuItem{
			{toggleLabel(i18n.T(i18n.TUIOverlayEnabled), m.overlayEnabled), "overlay-toggle"},
			{i18n.T(i18n.OutputDisplays), "overlay-displays"},
			{i18n.T(i18n.TUIOverlaySettings), "overlay-settings"},
			{i18n.T(i18n.OutputEvents), "output-events"},
		}
	}
	return []menuItem{
		{i18n.T(i18n.OutputTTSToggle), "tts-toggle"},
		{i18n.T(i18n.OutputVoice), "tts-voice"},
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

func (m *Model) pickTTSVoice() tea.Cmd {
	m.choices = nil
	selected := 0
	for _, voice := range tts.Voices() {
		if voice.ID == m.config.TTS.Voice {
			selected = len(m.choices)
		}
		m.choices = append(m.choices, choice{i18n.T(voice.Name), voice.ID})
	}
	cmd := m.pick("tts-voice", i18n.T(i18n.OutputVoicePicker))
	m.selected = selected
	return cmd
}

func (m *Model) ttsVoiceName() string {
	id := m.config.TTS.Voice
	if id == "" {
		id = tts.DefaultSettings().Voice
	}
	for _, voice := range tts.Voices() {
		if voice.ID == id {
			return i18n.T(voice.Name)
		}
	}
	return id
}
