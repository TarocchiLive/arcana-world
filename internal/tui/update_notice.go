package tui

import (
	"context"
	"errors"
	"fmt"

	"arcana-world/internal/i18n"

	tea "charm.land/bubbletea/v2"
)

// UpdateResult 是一次版本检查的结果。
type UpdateResult struct {
	Latest string // 最新稳定版号，带 v 前缀
	URL    string // 对应发布页
	Newer  bool   // 是否高于当前版本
}

// UpdateCheck 查询 GitHub 上的最新稳定版；失败时返回本地化错误。
type UpdateCheck func(context.Context) (UpdateResult, error)

type updateCheckResultMsg struct {
	result UpdateResult
	err    error
	manual bool
}

// ConfigureVersion 在模型初始化前设置当前程序版本，供帮助页显示。
func (m *Model) ConfigureVersion(version string) error {
	if m.initialized {
		return errors.New("version must be configured before model initialization")
	}
	m.version = version
	return nil
}

// ConfigureUpdateCheck 在模型初始化前设置启动时的后台版本检查。
func (m *Model) ConfigureUpdateCheck(check UpdateCheck) error {
	if m.initialized {
		return errors.New("update check must be configured before model initialization")
	}
	m.updateCheck = check
	return nil
}

func (m *Model) versionLabel() string {
	if m.version == "" {
		return "dev"
	}
	return m.version
}

// checkUpdate 在后台查询一次；manual 为 true 时把进度和结果写入状态提示。
func (m *Model) checkUpdate(manual bool) tea.Cmd {
	if m.updateCheck == nil {
		if manual {
			m.warn(i18n.T(i18n.TUIUpdateUnavailable))
		}
		return nil
	}
	if m.updateChecking {
		if manual {
			m.setStatus(i18n.T(i18n.TUIUpdateChecking))
		}
		return nil
	}
	m.updateChecking = true
	if manual {
		m.setStatus(i18n.T(i18n.TUIUpdateChecking))
	}
	check, ctx := m.updateCheck, m.ctx
	return func() tea.Msg {
		result, err := check(ctx)
		return updateCheckResultMsg{result: result, err: err, manual: manual}
	}
}

// handleUpdateCheckResult 保存新版本；启动检查失败保持安静，手动检查显示结果。
func (m *Model) handleUpdateCheckResult(msg updateCheckResultMsg) tea.Cmd {
	m.updateChecking = false
	if msg.err != nil {
		logger.Info("update check failed", "manual", msg.manual, "err", msg.err)
		if !msg.manual {
			return nil
		}
		m.warn(msg.err.Error())
		return m.notificationCommand()
	}
	if msg.result.Newer {
		m.newRelease = msg.result
		logger.Info("update available", "version", msg.result.Latest)
		if msg.manual {
			m.setStatus(fmt.Sprintf(i18n.T(i18n.TUIUpdateFound), msg.result.Latest, msg.result.URL))
		}
	} else if msg.manual {
		m.setStatus(fmt.Sprintf(i18n.T(i18n.TUIUpdateLatest), m.versionLabel(), msg.result.Latest))
	}
	if msg.manual {
		return m.notificationCommand()
	}
	return nil
}

// updateHint 是标题旁的新版本提示，无更新时为空。
func (m *Model) updateHint() string {
	if m.newRelease.Latest == "" {
		return ""
	}
	return m.theme.warning.Render(fmt.Sprintf(i18n.T(i18n.TUIUpdateAvailable), m.newRelease.Latest))
}
