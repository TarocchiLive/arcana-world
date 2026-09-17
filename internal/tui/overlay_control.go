package tui

import (
	"context"
	"errors"
	"fmt"

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	tea "github.com/charmbracelet/bubbletea"
)

type overlayRuntime struct {
	ctx       context.Context
	cancel    context.CancelFunc
	options   overlay.Options
	manager   *overlay.Manager
	state     string
	lastError error
	last      overlay.Config
	published bool
	started   chan struct{}
	startDone chan struct{}
	created   *overlay.Manager
}
type overlaySummary struct {
	title      string
	room, live bool
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

// ConfigureOverlay configures the optional native process before Init; enabling is session-local.
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
	return func() tea.Msg {
		close(owner.started)
		defer close(owner.startDone)
		manager, err := overlay.Start(owner.ctx, options)
		owner.created = manager
		return overlayStartedMsg{owner: owner, manager: manager, err: err}
	}
}
func (m *Model) stopOverlay() tea.Cmd {
	owner := m.overlay
	if owner == nil || owner.state == "off" {
		return nil
	}
	if owner.state == "failed" {
		owner.state = "off"
		return nil
	}
	if owner.state == "stopping" {
		return nil
	}
	owner.state = "stopping"
	owner.cancel()
	// Startup owns cleanup until its result arrives. Running processes have a Done watcher.
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
		m.log(fmt.Sprintf(i18n.T(i18n.TUILogOverlayStartupFailed), msg.err))
		return nil
	}
	owner.manager, owner.state = msg.manager, "running"
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
	owner.manager = nil
	owner.state = "off"
	if !requested {
		owner.state = "failed"
		if msg.err == nil {
			msg.err = errors.New(i18n.T(i18n.TUIOverlayUnexpectedExit))
		}
	}
	if msg.err != nil {
		owner.lastError = msg.err
		m.log(fmt.Sprintf(i18n.T(i18n.TUILogOverlayStopped), msg.err))
	}
	// Only an explicit enable received while stopping may start a successor.
	if requested && m.overlayEnabled {
		return m.startOverlay()
	}
	return nil
}
func (m *Model) publishOverlay() {
	owner := m.overlay
	if owner == nil || owner.manager == nil || owner.state != "running" {
		return
	}
	cfg := owner.options.Config
	cfg.Text = m.overlayContentText()
	if owner.published && owner.last == cfg {
		return
	}
	owner.last, owner.published = cfg, true
	if err := owner.manager.SetConfig(cfg); err != nil {
		owner.lastError = err
		m.log(fmt.Sprintf(i18n.T(i18n.TUILogOverlayUpdateFailed), err))
	}
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
		// A queued command sees cancellation before it can spawn a child.
	}
	return nil
}
