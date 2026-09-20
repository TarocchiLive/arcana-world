package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	tea "github.com/charmbracelet/bubbletea"
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
		// A display name is one picker row, even if the OS supplies control characters.
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
		return tea.DisableMouse
	}
	m.pick("overlay-displays", i18n.T(i18n.OutputDisplays))
	return tea.EnableMouseCellMotion
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

func (m *Model) overlayDisplayMouse(msg tea.MouseMsg) tea.Cmd {
	if m.busy || m.mode != "pick" || m.editKind != "overlay-displays" {
		return nil
	}
	if msg.Button == tea.MouseButtonWheelUp {
		m.selected = max(0, m.selected-1)
		return nil
	}
	if msg.Button == tea.MouseButtonWheelDown {
		m.selected = min(len(m.choices)-1, m.selected+1)
		return nil
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft || msg.X < 2 || msg.X >= 2+m.view.Width {
		return nil
	}
	row := msg.Y - m.pickerTop + m.view.YOffset - 2
	start := max(0, m.selected-m.pickerWindow()+1)
	if row < 0 || row >= m.pickerWindow() || start+row >= len(m.choices) || msg.Y < m.pickerTop || msg.Y >= m.pickerTop+m.view.Height {
		return nil
	}
	m.selected = start + row
	return m.choose()
}
