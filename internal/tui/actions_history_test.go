package tui

import (
	"reflect"
	"testing"

	"arcana-world/internal/domain"
)

func TestPrependRecentKeepsUnrelatedDuplicatesAndInput(t *testing.T) {
	titles := []string{"old", "chosen", "old", "tail"}
	got := prependRecent(titles, "chosen", 4, func(s string) string { return s })
	if !reflect.DeepEqual(got, []string{"chosen", "old", "old", "tail"}) {
		t.Fatalf("history order or unrelated duplicates changed: %v", got)
	}
	got[1] = "changed"
	if !reflect.DeepEqual(titles, []string{"old", "chosen", "old", "tail"}) {
		t.Fatal("new history aliases the input")
	}
	areas := []domain.Area{{ID: 1, Name: "old name"}, {ID: 2}, {ID: 2}, {ID: 3}}
	updated := domain.Area{ID: 1, Name: "new name"}
	recent := prependRecent(areas, updated, 3, func(a domain.Area) int64 { return a.ID })
	if !reflect.DeepEqual(recent, []domain.Area{updated, {ID: 2}, {ID: 2}}) {
		t.Fatalf("area identity or truncation changed: %v", recent)
	}
	if areas[0].Name != "old name" {
		t.Fatal("area input was modified")
	}
}
