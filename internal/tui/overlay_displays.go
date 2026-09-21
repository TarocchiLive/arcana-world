package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	tea "charm.land/bubbletea/v2"
)

func (m *Model) pickOverlayDisplays() tea.Cmd {
	options := overlay.Options{Config: m.config.Overlay.Config("")}
	if m.overlay != nil {
		options = m.overlay.options
	}
	return work(m, operation[[]overlay.Display]{label: i18n.OutputDisplays, discardOnCancel: true, handle: (*Model).handleOverlayDisplays}, func(ctx context.Context) ([]overlay.Display, error) {
		return overlay.ListDisplays(ctx, options)
	})
}

func (m *Model) handleOverlayDisplays(displays []overlay.Display, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	m.overlayDisplays = displays
	m.choices = nil
	for _, display := range displays {
		// 系统提供的名称即使含控制字符，也只能占一个选项行。
		name := strings.ReplaceAll(clean(display.Name), "\n", " ")
		m.choices = append(m.choices, choice{name, display.ID})
	}
	for _, id := range m.config.Overlay.Displays {
		if !slices.ContainsFunc(displays, func(display overlay.Display) bool { return display.ID == id }) {
			name := strings.ReplaceAll(clean(id), "\n", " ")
			m.choices = append(m.choices, choice{fmt.Sprintf(i18n.T(i18n.OutputDisplayDisconnected), name), id})
		}
	}
	if len(m.choices) == 0 {
		m.mode = ""
		m.log(i18n.T(i18n.TUIStatusNoEntries))
		return nil
	}
	return m.pick("overlay-displays", i18n.T(i18n.OutputDisplays))
}

func (m *Model) selectedOverlayDisplays() []string {
	if m.config.Overlay.Displays != nil {
		return slices.Clone(m.config.Overlay.Displays)
	}
	ids := make([]string, 0, len(m.overlayDisplays))
	for _, display := range m.overlayDisplays {
		if display.Selected {
			ids = append(ids, display.ID)
		}
	}
	return ids
}

func (m *Model) overlayDisplayLabel(ch choice) string {
	selected := slices.Contains(m.config.Overlay.Displays, ch.value)
	if m.config.Overlay.Displays == nil {
		selected = slices.ContainsFunc(m.overlayDisplays, func(display overlay.Display) bool { return display.ID == ch.value && display.Selected })
	}
	mark := "[ ]"
	if selected {
		mark = "[x]"
	}
	return fmt.Sprintf(i18n.T(i18n.OutputDisplayEnabled), mark, ch.label)
}

func (m *Model) toggleOverlayDisplay(id string) tea.Cmd {
	settings := m.config.Overlay
	ids := m.selectedOverlayDisplays()
	if slices.Contains(ids, id) {
		ids = slices.DeleteFunc(ids, func(value string) bool { return value == id })
	} else {
		ids = append(ids, id)
	}
	settings.Displays = ids
	return work(m, overlayConfigOperation(), func(ctx context.Context) (struct{}, error) {
		if err := ctx.Err(); err != nil {
			return struct{}{}, err
		}
		cfg := m.store.Config()
		cfg.Overlay = settings
		return struct{}{}, m.store.SaveConfig(cfg)
	})
}
