package tui

import (
	"context"

	"arcana-world/internal/bili"
	"arcana-world/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) resetSettings() tea.Cmd {
	account := m.account
	return m.work("settings-reset", func(ctx context.Context) (any, error) {
		if err := m.acquireOBS(ctx); err != nil {
			return nil, err
		}
		defer func() { <-m.obsLifecycle }()
		client, err := bili.New(store.DefaultConfig().Proxy)
		if err != nil {
			return nil, err
		}
		if account != nil {
			client.SetAccount(*account)
		}
		if _, err := m.store.ResetSettings(); err != nil {
			client.HTTP.CloseIdleConnections()
			return nil, err
		}
		return client, nil
	})
}
