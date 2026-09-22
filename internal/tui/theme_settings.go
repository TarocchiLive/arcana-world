package tui

import (
	"context"

	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
)

func themeLabel(id string) string {
	switch id {
	case "midnight":
		return i18n.T(i18n.LumenThemeMidnight)
	case "nord":
		return i18n.T(i18n.LumenThemeNord)
	case "ember":
		return i18n.T(i18n.LumenThemeEmber)
	case "paper":
		return i18n.T(i18n.LumenThemePaper)
	default:
		return i18n.T(i18n.LumenThemeLumen)
	}
}

func (m *Model) themeName() string {
	return themeLabel(m.config.TUITheme)
}

func (m *Model) pickTheme() tea.Cmd {
	m.choices = []choice{
		{themeLabel("lumen"), "lumen"},
		{themeLabel("midnight"), "midnight"},
		{themeLabel("nord"), "nord"},
		{themeLabel("ember"), "ember"},
		{themeLabel("paper"), "paper"},
	}
	cmd := m.pick("theme", i18n.T(i18n.LumenTheme))
	for index, item := range m.choices {
		if item.value == m.config.TUITheme {
			m.selected = index
			break
		}
	}
	return cmd
}

func (m *Model) appearanceChoices() []choice {
	return []choice{
		{toggleLabel(i18n.T(i18n.LumenMotion), !m.config.TUIMotionDisabled), "motion"},
		{toggleLabel(i18n.T(i18n.LumenCompactHeader), m.config.TUICompactHeader), "compact-header"},
		{toggleLabel(i18n.T(i18n.LumenWarningsOnly), m.config.TUINotificationsWarningsOnly), "warnings-only"},
	}
}

func (m *Model) pickAppearance() tea.Cmd {
	m.choices = m.appearanceChoices()
	return m.pick("appearance", i18n.T(i18n.LumenAppearance))
}

func (m *Model) toggleAppearance(id string) tea.Cmd {
	cfg := m.config
	switch id {
	case "motion":
		cfg.TUIMotionDisabled = !cfg.TUIMotionDisabled
	case "compact-header":
		cfg.TUICompactHeader = !cfg.TUICompactHeader
	case "warnings-only":
		cfg.TUINotificationsWarningsOnly = !cfg.TUINotificationsWarningsOnly
	default:
		return nil
	}
	return work(m, appearanceOperation(), func(ctx context.Context) (struct{}, error) {
		if err := m.session.Lock(ctx); err != nil {
			return struct{}{}, err
		}
		defer m.session.Unlock()
		// 获得会话锁后再次检查取消，避免等待中的保存落盘。
		if err := ctx.Err(); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, m.store.SaveConfig(cfg)
	})
}
