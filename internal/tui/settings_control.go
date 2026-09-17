package tui

import (
	"context"

	"arcana-world/internal/bili"
	"arcana-world/internal/store"
	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) resetSettings() tea.Cmd {
	account := m.account
	return work(m, settingsResetOperation(), func(ctx context.Context) (*bili.Client, error) {
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
	op := obsURLOperation
	if kind == "obs-password" {
		op = obsPasswordOperation
	}
	return work(m, op, func(ctx context.Context) (struct{}, error) {
		if err := m.session.Lock(ctx); err != nil {
			return struct{}{}, err
		}
		defer m.session.Unlock()
		if err := save(); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, m.obsClient.Disconnect()
	})
}
