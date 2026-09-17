package tui

import (
	"slices"

	"arcana-world/internal/i18n"
	"arcana-world/internal/presentation"
	"arcana-world/internal/tts"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) outputMenu() []menuItem {
	var items []menuItem
	disabled := m.config.OverlayDisabledEvents
	if m.page == overlayPage {
		items = []menuItem{
			{toggleLabel(i18n.T(i18n.TUIOverlayEnabled), m.overlayEnabled), "overlay-toggle"},
			{i18n.T(i18n.TUIOverlaySettings), "overlay-settings"},
		}
	} else {
		disabled = m.config.TTS.DisabledEvents
		items = []menuItem{
			{"启用 / 停用语音播报", "tts-toggle"},
			{"停止当前播放并清空队列", "tts-stop"},
			{"试听当前音色", "tts-preview"},
		}
	}
	for _, spec := range presentation.Specs() {
		mark := "[x] "
		if slices.Contains(disabled, spec.ID) {
			mark = "[ ] "
		}
		items = append(items, menuItem{mark + spec.Label, "output-event:" + spec.ID})
	}
	return items
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
		m.choices = append(m.choices, choice{voice.Name, voice.ID})
	}
	cmd := m.pick("tts-voice", "选择 TTS 音色")
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
			return voice.Name
		}
	}
	return id
}
