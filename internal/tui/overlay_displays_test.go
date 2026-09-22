package tui

import (
	"context"
	"slices"
	"strings"
	"testing"

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestOverlayDisplayMouseSelectionPersistsEmpty(t *testing.T) {
	for _, width := range []int{56, 120} {
		m := lifecycleModel(t, context.Background())
		m.page = overlayPage
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		m.handleOverlayDisplays([]overlay.Display{{ID: "first", Name: "First", Selected: true}, {ID: "second", Name: "Second"}}, nil, i18n.OutputDisplays)
		rendered := strings.Split(ansi.Strip(m.View().Content), "\n")
		secondRow, secondColumn := -1, -1
		for row, line := range rendered {
			if column := strings.Index(line, "Second"); column >= 0 {
				secondRow, secondColumn = row, ansi.StringWidth(line[:column])
			}
		}
		if secondRow < 0 {
			t.Fatal("second display is not visible")
		}
		// 导航栏与工作区内边距不能成为选择器的点击目标。
		if _, cmd := m.Update(tea.MouseClickMsg{X: 2, Y: secondRow, Button: tea.MouseLeft}); cmd != nil {
			t.Fatal("click outside the workspace toggled a display")
		}
		_, cmd := m.Update(tea.MouseClickMsg{X: secondColumn, Y: secondRow, Button: tea.MouseLeft})
		if cmd == nil {
			t.Fatal("click did not toggle a display")
		}
		m.Update(cmd())
		if ids := m.store.Config().Overlay.Displays; !slices.Equal(ids, []string{"first", "second"}) {
			t.Fatalf("width %d: clicked wrong display: %v", width, ids)
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
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	rendered := ansi.Strip(m.View().Content)
	if !strings.Contains(rendered, "[x]") || strings.Contains(m.choices[1].label, "\n") {
		t.Fatalf("disconnected selection is not a checked single row: %q", rendered)
	}
	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if cmd == nil {
		t.Fatal("keyboard did not toggle disconnected display")
	}
	m.Update(cmd())
	if ids := m.store.Config().Overlay.Displays; ids == nil || len(ids) != 0 {
		t.Fatalf("disconnected display was not removed: %#v", ids)
	}
}
