package tui

import (
	"context"
	"errors"
	"fmt"

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	tea "github.com/charmbracelet/bubbletea"
)

// 业务模型只持有纯 Go 管理器，不引用原生图形后端。
// 只发布公开的房间标题和开播状态，绝不把登录令牌、推流地址或密钥送到浮层。
type overlayRuntime struct {
	ctx       context.Context
	cancel    context.CancelFunc
	options   overlay.Options
	manager   *overlay.Manager
	last      overlaySummary
	published bool
	// started 在命令真正执行时关闭；created 仅在 startDone 关闭后读取。
	started   chan struct{}
	startDone chan struct{}
	created   *overlay.Manager
}
type overlaySummary struct {
	title string
	room  bool
	live  bool
}
type overlayStartedMsg struct {
	owner   *overlayRuntime
	manager *overlay.Manager
	err     error
}
type overlayStoppedMsg struct {
	owner *overlayRuntime
	err   error
}

// EnableOverlay 在 Init 前启用可选浮层；原生启动在 Bubble Tea 命令中进行，
// 不阻塞 TUI，失败通过现有日志呈现，不自动重试或回退到普通窗口。
func (m *Model) EnableOverlay(options overlay.Options) error {
	if m.initialized || m.overlay != nil {
		return errors.New("overlay must be enabled once before model initialization")
	}
	cfg, err := options.Config.Normalize()
	if err != nil {
		return err
	}
	options.Config = cfg
	ctx, cancel := context.WithCancel(m.ctx)
	m.overlay = &overlayRuntime{ctx: ctx, cancel: cancel, options: options, started: make(chan struct{}), startDone: make(chan struct{})}
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
	state := i18n.T(i18n.TUILiveOffline)
	if summary.live {
		state = i18n.T(i18n.TUILiveOnline)
	}
	return "Arcana World\n" + clean(summary.title) + "\n" + state
}
func (m *Model) startOverlay() tea.Cmd {
	owner := m.overlay
	if owner == nil {
		return nil
	}
	options := owner.options
	options.Config.Text = overlayText(m.overlaySummary())
	return func() tea.Msg {
		close(owner.started)
		defer close(owner.startDone)
		manager, err := overlay.Start(owner.ctx, options)
		owner.created = manager
		return overlayStartedMsg{owner: owner, manager: manager, err: err}
	}
}
func (m *Model) handleOverlayStarted(msg overlayStartedMsg) tea.Cmd {
	if msg.owner != m.overlay {
		if msg.manager != nil {
			_ = msg.manager.Close()
		}
		return nil
	}
	if msg.err != nil {
		m.log(fmt.Sprintf(i18n.T(i18n.TUILogOverlayStartupFailed), msg.err))
		return nil
	}
	msg.owner.manager = msg.manager
	m.log(i18n.T(i18n.TUILogOverlayStarted))
	// 启动期间业务状态可能已变化，发布当前快照，而非重放过期事件。
	m.publishOverlay()
	return func() tea.Msg {
		<-msg.manager.Done()
		return overlayStoppedMsg{owner: msg.owner, err: msg.manager.Err()}
	}
}
func (m *Model) handleOverlayStopped(msg overlayStoppedMsg) {
	if msg.owner != m.overlay {
		return
	}
	msg.owner.manager = nil
	if msg.err != nil {
		m.log(fmt.Sprintf(i18n.T(i18n.TUILogOverlayStopped), msg.err))
	}
}
func (m *Model) publishOverlay() {
	owner := m.overlay
	if owner == nil || owner.manager == nil {
		return
	}
	summary := m.overlaySummary()
	if owner.published && owner.last == summary {
		return
	}
	owner.last, owner.published = summary, true
	if err := owner.manager.SetText(overlayText(summary)); err != nil {
		m.log(fmt.Sprintf(i18n.T(i18n.TUILogOverlayUpdateFailed), err))
	}
}
func (m *Model) closeOverlay() error {
	if m.overlay == nil {
		return nil
	}
	// 启动结果可能还未送回 TUI。先取消，再等待已经执行的 Start 收尾，
	// 防止主进程退出时终止回收协程，留下尚未连接的原生子进程。
	owner := m.overlay
	owner.cancel()
	select {
	case <-owner.started:
		<-owner.startDone
		if owner.created != nil {
			return owner.created.Close()
		}
	default:
		// 尚未执行的命令随后只会看到已取消的上下文，不会创建子进程。
	}
	return nil
}
