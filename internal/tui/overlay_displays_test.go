package tui

import (
	"context"
	"slices"
	"strings"
	"testing"

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestOverlayDisplayMouseSelectionPersistsEmpty(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.page = overlayPage
	m.Update(tea.WindowSizeMsg{Width: 56, Height: 24})
	m.handleOverlayDisplays([]overlay.Display{{ID: "first", Name: "First", Selected: true}, {ID: "second", Name: "Second"}}, nil, i18n.OutputDisplays)
	rendered := strings.Split(ansi.Strip(m.View()), "\n")
	secondRow := -1
	for row, line := range rendered {
		if strings.Contains(line, "Second") {
			secondRow = row
		}
	}
	if secondRow < 0 {
		t.Fatal("second display is not visible")
	}
	// With wrapped tabs, a click still targets the actual second display row.
	_, cmd := m.Update(tea.MouseMsg{X: 6, Y: secondRow, Button: tea.MouseButtonLeft, Action: tea.MouseActionPress})
	if cmd == nil {
		t.Fatal("click did not toggle a display")
	}
	m.Update(cmd())
	if ids := m.store.Config().Overlay.Displays; !slices.Equal(ids, []string{"first", "second"}) {
		t.Fatalf("clicked wrong display: %v", ids)
	}
	for _, id := range []string{"first", "second"} {
		m.Update(m.toggleOverlayDisplay(id)())
	}
	cfg := m.store.Config()
	if cfg.Overlay.Displays == nil || len(cfg.Overlay.Displays) != 0 {
		t.Fatalf("deselecting all reverted to the default display: %#v", cfg.Overlay.Displays)
	}
	if m.mode != "pick" || m.editKind != "overlay-displays" {
		t.Fatal("toggle closed the display picker")
	}
	selection, err := cfg.Overlay.Config("").SelectedDisplays()
	if err != nil || selection == nil || len(selection) != 0 {
		t.Fatalf("empty selection did not reach native config: %#v, %v", selection, err)
	}
}

func TestOverlayDisplaySelectionSurvivesRefreshFailure(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.page = overlayPage
	m.Update(tea.WindowSizeMsg{Width: 56, Height: 24})
	m.config.Overlay.Displays = []string{"disconnected\nscreen"}
	m.handleOverlayDisplays([]overlay.Display{{ID: "connected", Name: strings.Repeat("Display", 30)}}, nil, i18n.OutputDisplays)
	m.handleOverlayDisplays(nil, context.DeadlineExceeded, i18n.OutputDisplays)
	if m.mode != "pick" || len(m.choices) != 2 {
		t.Fatal("failed refresh discarded the editable selection")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	rendered := ansi.Strip(m.View())
	if !strings.Contains(rendered, "[x]") || strings.Contains(m.choices[1].label, "\n") {
		t.Fatalf("disconnected selection is not a checked single row: %q", rendered)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	if cmd == nil {
		t.Fatal("keyboard did not toggle disconnected display")
	}
	m.Update(cmd())
	if ids := m.store.Config().Overlay.Displays; ids == nil || len(ids) != 0 {
		t.Fatalf("disconnected display was not removed: %#v", ids)
	}
}

func TestOverlayDisplayBusyCancelDisablesMouse(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.handleOverlayDisplays([]overlay.Display{{ID: "display", Name: "Display"}}, nil, i18n.OutputDisplays)
	_ = m.toggleOverlayDisplay("display")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.mode != "" || cmd == nil {
		t.Fatal("cancel did not leave the picker and restore terminal mouse input")
	}
	if got, want := cmd(), tea.DisableMouse(); got != want {
		t.Fatalf("cancel command = %T, want %T", got, want)
	}
}
