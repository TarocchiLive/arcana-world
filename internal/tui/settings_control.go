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
		if err := m.session.Lock(ctx); err != nil {
			return nil, err
		}
		defer m.session.Unlock()
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
		m.session.Track(client)
		return client, nil
	})
}

// OBS settings are committed under the session gate before disconnecting.
func (m *Model) saveOBSSetting(kind string, save func() error) tea.Cmd {
	return m.work(kind, func(ctx context.Context) (any, error) {
		if err := m.session.Lock(ctx); err != nil {
			return nil, err
		}
		defer m.session.Unlock()
		if err := save(); err != nil {
			return nil, err
		}
		return nil, m.obsClient.Disconnect()
	})
}
