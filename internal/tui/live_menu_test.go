package tui

import (
	"context"
	"testing"

	"arcana-world/internal/app"
	"arcana-world/internal/domain"
	"arcana-world/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func TestLiveMenuFollowsRoomResultsAndKeepsSelection(t *testing.T) {
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.Config()
	cfg.ExitLiveStopDisabled = true
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	m.account = &domain.Account{UID: "1"}

	assertMenu := func(t *testing.T, want string) {
		t.Helper()
		counts := make(map[string]int)
		for _, item := range m.menu() {
			counts[item.action]++
		}
		if counts[want] != 1 || counts["start-confirm"]+counts["stop-confirm"] != 1 {
			t.Fatalf("expected only %s as the live action, got %v", want, counts)
		}
		if counts["refresh"] != 1 || counts["reveal"] != 1 {
			t.Fatalf("live state lost refresh or reveal: %v", counts)
		}
	}
	assertMenu(t, "start-confirm")
	for index, item := range m.menu() {
		if item.action == "start-confirm" {
			m.cursors[livePage] = index
		}
	}

	for _, step := range []struct {
		name   string
		result taskMessage
		want   string
	}{
		{"offline refresh", taskResult[domain.Room]{operation: refreshOperation(), value: domain.Room{ID: 1, AreaID: 1}}, "start"},
		{"start", taskResult[app.StartOutcome]{operation: startOperation(), value: app.StartOutcome{}}, "stop"},
		{"stop", taskResult[app.StopOutcome]{operation: stopOperation(), value: app.StopOutcome{}}, "start"},
		{"online refresh", taskResult[domain.Room]{operation: refreshOperation(), value: domain.Room{ID: 1, AreaID: 1, Live: true}}, "stop"},
		{"offline refresh after online", taskResult[domain.Room]{operation: refreshOperation(), value: domain.Room{ID: 1, AreaID: 1}}, "start"},
	} {
		t.Run(step.name, func(t *testing.T) {
			m.Update(step.result)
			assertMenu(t, step.want+"-confirm")
			items := m.menu()
			cursor := m.cursors[livePage]
			if cursor < 0 || cursor >= len(items) || items[cursor].action != step.want+"-confirm" {
				t.Fatalf("live action selection did not survive %s: cursor=%d, menu=%v", step.name, cursor, items)
			}
			m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if m.mode != "confirm" || m.confirmAction != step.want {
				t.Fatalf("selected action opened %q/%q, want confirmation for %s", m.mode, m.confirmAction, step.want)
			}
			m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		})
	}
}
