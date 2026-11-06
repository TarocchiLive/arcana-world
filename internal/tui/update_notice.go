package tui

import (
	"context"
	"errors"
	"fmt"

	"arcana-world/internal/i18n"

	tea "charm.land/bubbletea/v2"
)

// UpdateCheck 返回比当前版本更高的稳定版号及发布页；无更新时版本为空。
type UpdateCheck func(context.Context) (version, url string, err error)

type updateAvailableMsg struct{ version, url string }

// ConfigureUpdateCheck 在模型初始化前设置启动时的后台版本检查。
func (m *Model) ConfigureUpdateCheck(check UpdateCheck) error {
	if m.initialized {
		return errors.New("update check must be configured before model initialization")
	}
	m.updateCheck = check
	return nil
}

// checkUpdate 在后台查询一次；失败只记日志，不打扰界面。
func (m *Model) checkUpdate() tea.Cmd {
	if m.updateCheck == nil {
		return nil
	}
	check, ctx := m.updateCheck, m.ctx
	return func() tea.Msg {
		version, url, err := check(ctx)
		if err != nil {
			logger.Info("startup update check failed", "err", err)
			return nil
		}
		if version == "" {
			return nil
		}
		logger.Info("update available", "version", version)
		return updateAvailableMsg{version: version, url: url}
	}
}

// updateHint 是标题旁的新版本提示，无更新时为空。
func (m *Model) updateHint() string {
	if m.newRelease.version == "" {
		return ""
	}
	return m.theme.warning.Render(fmt.Sprintf(i18n.T(i18n.TUIUpdateAvailable), m.newRelease.version))
}
