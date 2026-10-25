package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	tea "charm.land/bubbletea/v2"
)

type overlayRuntime struct {
	ctx          context.Context
	cancel       context.CancelFunc
	options      overlay.Options
	manager      *overlay.Manager
	state        string
	lastError    error
	last         overlay.Config
	published    bool
	preview      bool
	previewText  string
	previewRoles string
	previewNext  time.Time
	started      chan struct{}
	startDone    chan struct{}
	created      *overlay.Manager
}
type overlaySummary struct {
	title      string
	room, live bool
}
type overlayStartedMsg struct {
	owner   *overlayRuntime
	manager *overlay.Manager
	err     error
	elapsed time.Duration
}
type overlayStoppedMsg struct {
	owner *overlayRuntime
	err   error
}

// ConfigureOverlay 在 Init 前配置原生子进程；开关仅影响本次会话。
func (m *Model) ConfigureOverlay(options overlay.Options, enabled bool) error {
	if m.initialized || m.overlay != nil {
		return errors.New("overlay must be configured once before model initialization")
	}
	cfg, err := options.Config.Normalize()
	if err != nil {
		return err
	}
	options.Config = cfg
	m.overlay = &overlayRuntime{options: options, state: "off"}
	m.overlayEnabled = enabled
	return nil
}
func (m *Model) overlaySummary() overlaySummary {
	if m.room == nil {
		return overlaySummary{}
	}
	return overlaySummary{title: m.room.Title, room: true, live: m.room.Live}
}
func overlayText(summary overlaySummary) string {
	if !summary.room {
		return "Arcana World"
	}
	state := "未开播"
	if summary.live {
		state = "直播中"
	}
	return "Arcana World\n" + clean(summary.title) + "\n" + state
}
func (m *Model) startOverlay() tea.Cmd {
	if !m.overlayEnabled || m.overlay == nil || (m.overlay.state != "off" && m.overlay.state != "failed") {
		return nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	owner := &overlayRuntime{ctx: ctx, cancel: cancel, options: m.overlay.options, state: "starting", started: make(chan struct{}), startDone: make(chan struct{})}
	m.overlay = owner
	options := owner.options
	options.Config.Text = m.overlayContentText()
	options.Config.TextRoles = m.overlayChat.contentRoles
	options.Config = m.previewOverlayAppearance(options.Config)
	logger.Debug("overlay start requested", "op", "overlay start")
	return func() tea.Msg {
		close(owner.started)
		defer close(owner.startDone)
		began := time.Now()
		manager, err := overlay.Start(owner.ctx, options)
		owner.created = manager
		return overlayStartedMsg{owner: owner, manager: manager, err: err, elapsed: time.Since(began)}
	}
}
func (m *Model) stopOverlay() tea.Cmd {
	owner := m.overlay
	if owner == nil || owner.state == "off" {
		return nil
	}
	owner.preview = false
	if owner.state == "failed" {
		owner.state = "off"
		return nil
	}
	if owner.state == "stopping" {
		return nil
	}
	owner.state = "stopping"
	owner.cancel()
	// 启动结果到达前由启动流程清理；运行后由 Done 监听回收。
	manager := owner.manager
	if manager == nil {
		return nil
	}
	return func() tea.Msg { return overlayStoppedMsg{owner: owner, err: manager.Close()} }
}
func (m *Model) handleOverlayStarted(msg overlayStartedMsg) tea.Cmd {
	if msg.owner != m.overlay {
		if msg.manager != nil {
			return func() tea.Msg { _ = msg.manager.Close(); return nil }
		}
		return nil
	}
	owner := msg.owner
	if owner.state == "stopping" || !m.overlayEnabled {
		if msg.manager != nil {
			return func() tea.Msg { return overlayStoppedMsg{owner: owner, err: msg.manager.Close()} }
		}
		return m.handleOverlayStopped(overlayStoppedMsg{owner: owner})
	}
	if msg.err != nil {
		owner.cancel()
		owner.state, owner.lastError = "failed", msg.err
		logger.Error("overlay start failed", "op", "overlay start", "elapsed", msg.elapsed, "err", msg.err)
		// 浮层错误已是完整的本地化提示，不再套用前缀模板。
		m.warn(msg.err.Error())
		return nil
	}
	owner.manager, owner.state = msg.manager, "running"
	logger.Info("overlay started", "op", "overlay start", "elapsed", msg.elapsed)
	m.log(i18n.T(i18n.TUILogOverlayStarted))
	m.publishOverlay()
	return func() tea.Msg { <-msg.manager.Done(); return overlayStoppedMsg{owner: owner, err: msg.manager.Err()} }
}
func (m *Model) handleOverlayStopped(msg overlayStoppedMsg) tea.Cmd {
	if msg.owner != m.overlay || (msg.owner.state != "stopping" && msg.owner.state != "running") {
		return nil
	}
	owner := msg.owner
	owner.cancel()
	requested := owner.state == "stopping"
	owner.preview = false
	owner.manager = nil
	owner.state = "off"
	if !requested {
		owner.state = "failed"
		logger.Error("overlay exited unexpectedly", "op", "overlay run", "err", msg.err)
		if msg.err == nil {
			msg.err = errors.New(i18n.T(i18n.TUIOverlayUnexpectedExit))
		}
	} else if msg.err != nil {
		logger.Error("overlay stop failed", "op", "overlay stop", "err", msg.err)
	} else {
		logger.Info("overlay stopped")
	}
	if msg.err != nil {
		owner.state = "failed"
		owner.lastError = msg.err
		m.warn(msg.err.Error())
	}
	// 只有停止期间明确收到启用请求，才启动下一进程。
	if requested && m.overlayEnabled && msg.err == nil {
		return m.startOverlay()
	}
	return nil
}
func (m *Model) publishOverlay() {
	owner := m.overlay
	if owner != nil && ((!m.web.enabled && m.page != overlayPage) || !m.overlayEnabled) {
		owner.preview = false
	}
	if owner == nil || owner.manager == nil || owner.state != "running" {
		return
	}
	cfg := owner.options.Config
	if owner.preview {
		// 示例只覆盖展示快照，不进入弹幕源、历史或语音播报路径。
		cfg.Text, cfg.TextRoles = owner.previewContent(time.Now())
	} else {
		cfg.Text = m.overlayContentText()
		cfg.TextRoles = m.overlayChat.contentRoles
	}
	cfg = m.previewOverlayAppearance(cfg)
	if owner.published && owner.last == cfg {
		return
	}
	owner.last, owner.published = cfg, true
	if err := owner.manager.SetConfig(cfg); err != nil {
		owner.lastError = err
		logger.Warn("overlay config update failed", "err", err)
		m.warn(fmt.Sprintf(i18n.T(i18n.TUILogOverlayUpdateFailed), err))
	}
}

func (owner *overlayRuntime) previewContent(now time.Time) (string, string) {
	const interval = 1500 * time.Millisecond
	if owner.previewText == "" {
		owner.previewText = i18n.T(i18n.TUIOverlayPreviewText)
		owner.previewRoles = "ncagsnncnn"
		owner.previewNext = now.Add(interval)
	} else if !now.Before(owner.previewNext) {
		// 复用业务轮询，每次把最旧示例移到末尾，触发与新弹幕相同的滚动动效。
		first, rest, ok := strings.Cut(owner.previewText, "\n")
		if ok {
			owner.previewText = rest + "\n" + first
			owner.previewRoles = owner.previewRoles[1:] + owner.previewRoles[:1]
		}
		owner.previewNext = now.Add(interval)
	}
	return owner.previewText, owner.previewRoles
}
func (m *Model) closeOverlay() error {
	owner := m.overlay
	if owner == nil || owner.cancel == nil {
		return nil
	}
	owner.cancel()
	select {
	case <-owner.started:
		<-owner.startDone
		if owner.created != nil {
			return owner.created.Close()
		}
	default:
		// 排队中的启动命令会在创建子进程前检查取消状态。
	}
	return nil
}
