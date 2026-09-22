package tui

import (
	"strings"
	"testing"

	"arcana-world/internal/domain"
	tea "charm.land/bubbletea/v2"
)

func selectionKey(s *roomSelection, key rune) *selectionResult {
	result, _ := s.Update(tea.KeyPressMsg{Code: key})
	return result
}

func selectionType(s *roomSelection, text string) {
	for _, r := range text {
		s.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func TestTitleSelectionEmptyHistory(t *testing.T) {
	s := newTitleSelection("", nil)
	if result := selectionKey(s, tea.KeyEnter); result != nil {
		t.Fatal("empty title submitted")
	}
	selectionType(s, "新标题")
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Title != "新标题" {
		t.Fatalf("typed title: %+v", result)
	}
	s = newTitleSelection("当前标题", nil)
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Title != "当前标题" {
		t.Fatalf("current title: %+v", result)
	}
}

func TestTitleSelectionHistoryAndEditing(t *testing.T) {
	s := newTitleSelection("当前标题", []string{"旧标题", "另一个"})
	if !strings.Contains(s.View(100, 10), "旧标题") {
		t.Fatal("prefilled title hid unrelated history")
	}
	selectionKey(s, tea.KeyDown)
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Title != "旧标题" {
		t.Fatalf("history selection: %+v", result)
	}
	s = newTitleSelection("当前标题", []string{"旧标题"})
	selectionKey(s, tea.KeyDown)
	selectionKey(s, tea.KeyTab)
	selectionType(s, "更新")
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Title != "旧标题更新" {
		t.Fatalf("editable history: %+v", result)
	}
}

func TestSelectionPasteUsesEditedTitleAndSearch(t *testing.T) {
	title := newTitleSelection("", []string{"旧标题"})
	selectionKey(title, tea.KeyDown)
	title.Update(tea.PasteMsg{Content: "粘贴的新标题"})
	if result := selectionKey(title, tea.KeyEnter); result == nil || result.Title != "粘贴的新标题" {
		t.Fatalf("paste submitted history instead of edited title: %+v", result)
	}
	area := newAreaSelection(selectionCatalog(), nil, 12)
	area.Update(tea.PasteMsg{Content: "围棋"})
	if result := selectionKey(area, tea.KeyEnter); result == nil || result.Area == nil || result.Area.ID != 34 {
		t.Fatalf("pasted search did not select matching area: %+v", result)
	}
}

func selectionCatalog() []domain.Area {
	return []domain.Area{
		{ID: 12, Parent: "游戏", Name: "星际"},
		{ID: 34, Parent: "游戏", Name: "围棋"},
		{ID: 56, Parent: "生活", Name: "聊天"},
	}
}

func TestAreaSelectionSearchAndRuneBackspace(t *testing.T) {
	s := newAreaSelection(selectionCatalog(), nil, 12)
	selectionType(s, "围棋错")
	if result := selectionKey(s, tea.KeyEnter); result != nil {
		t.Fatal("unmatched search submitted")
	}
	selectionKey(s, tea.KeyBackspace)
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Area == nil || result.Area.ID != 34 {
		t.Fatalf("Unicode search: %+v", result)
	}
	s.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	selectionType(s, "56")
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Area == nil || result.Area.Parent != "生活" || result.Area.ID != 56 {
		t.Fatalf("ID search: %+v", result)
	}
	s.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	selectionType(s, "游戏")
	selectionKey(s, tea.KeyDown)
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Area == nil || result.Area.ID != 34 {
		t.Fatalf("parent search: %+v", result)
	}
}

func TestAreaSelectionHierarchyAndBack(t *testing.T) {
	s := newAreaSelection(selectionCatalog(), nil, 12)
	if result := selectionKey(s, tea.KeyRight); result != nil {
		t.Fatal("parent selection submitted")
	}
	selectionKey(s, tea.KeyDown)
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Area == nil || result.Area.ID != 34 {
		t.Fatalf("child selection: %+v", result)
	}
	if result := selectionKey(s, tea.KeyEsc); result != nil {
		t.Fatal("back from child canceled modal")
	}
	selectionKey(s, tea.KeyDown)
	selectionKey(s, tea.KeyEnter)
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Area == nil || result.Area.ID != 56 {
		t.Fatalf("second parent: %+v", result)
	}
	if result := selectionKey(s, tea.KeyLeft); result != nil {
		t.Fatal("left from child canceled modal")
	}
	if result := selectionKey(s, tea.KeyEsc); result == nil || !result.Canceled {
		t.Fatal("root escape did not cancel")
	}
}

func TestAreaSelectionRecentNormalizationAndDedup(t *testing.T) {
	all := selectionCatalog()
	s := newAreaSelection(all, []domain.Area{{ID: 34, Name: "旧名称"}, all[1], {ID: 999, Name: "已删除"}}, 34)
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Area == nil || *result.Area != all[1] {
		t.Fatalf("noncanonical recent: %+v", result)
	}
	selectionKey(s, tea.KeyDown)
	if result := selectionKey(s, tea.KeyEnter); result != nil {
		t.Fatal("duplicate or stale recent displaced first parent")
	}
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Area == nil || result.Area.ID != 12 {
		t.Fatalf("catalog after recent: %+v", result)
	}
}

func TestAreaSelectionSmallWindowNavigation(t *testing.T) {
	s := newAreaSelection(selectionCatalog(), nil, 0)
	selectionType(s, "游戏")
	s.resize(80, 4)
	selectionKey(s, tea.KeyPgDown)
	view := s.View(80, 4)
	if strings.Count(view, "\n") >= 4 || !strings.Contains(view, "围棋") {
		t.Fatalf("selected row not visible in bounded view: %q", view)
	}
	selectionKey(s, tea.KeyHome)
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Area == nil || result.Area.ID != 12 {
		t.Fatalf("home: %+v", result)
	}
	selectionKey(s, tea.KeyEnd)
	if result := selectionKey(s, tea.KeyEnter); result == nil || result.Area == nil || result.Area.ID != 34 {
		t.Fatalf("end: %+v", result)
	}
}
