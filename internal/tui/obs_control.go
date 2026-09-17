package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"arcana-world/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
)

const obsOperationTimeout = 30 * time.Second

type obsEventMsg struct{ closed bool }
type obsResultMsg struct {
	id   int
	kind string
	err  error
}

func toggleLabel(label string, enabled bool) string {
	if enabled {
		return i18n.T(i18n.TUIToggleOn) + label
	}
	return i18n.T(i18n.TUIToggleOff) + label
}

// Close 仅执行一次：按退出设置停止 OBS、关闭当前账号直播间，再释放资源。
func (m *Model) Close() error {
	m.closeOnce.Do(func() {
		m.session.BeginClose()
		if m.cancel != nil {
			m.cancel()
		}
		if m.obsCancel != nil {
			m.obsCancel()
		}
		stopErr := m.session.Close(m.client, m.room != nil && m.room.Live)
		if stopErr != nil {
			stopErr = errors.Join(stopErr, m.journal.Write(m.safe(stopErr.Error())))
		}
		m.closeErr = errors.Join(stopErr, m.closeOverlay(), m.closeChat(), m.obsClient.Close(), m.journal.Write(i18n.T(i18n.TUILogApplicationExited)), m.journal.Close())
		if m.clearDataOnExit {
			if m.closeErr != nil {
				m.closeErr = fmt.Errorf(i18n.T(i18n.TUISettingsClearDataShutdownFailed), m.closeErr)
				return
			}
			m.closeErr = m.store.ClearData()
		}
	})
	return m.closeErr
}

// 仅由一个命令消费连接事件；Init 防止重复进入。
func (m *Model) watchOBS() tea.Cmd {
	events := m.obsClient.Events()
	return func() tea.Msg {
		select {
		case _, ok := <-events:
			return obsEventMsg{closed: !ok}
		case <-m.ctx.Done():
			return obsEventMsg{closed: true}
		}
	}
}
func (m *Model) handleOBSEvent(msg obsEventMsg) tea.Cmd {
	previous := m.obsState
	// 排队事件可能属于已断开的旧连接，不能发布其过期状态。
	m.obsState = m.obsClient.Snapshot()
	if previous.Connected && !m.obsState.Connected && m.obsState.Err != nil {
		m.log(fmt.Sprintf(i18n.T(i18n.TUILogOBSConnectionLost), m.obsState.Err.Error()))
	}
	if msg.closed {
		return nil
	}
	return m.watchOBS()
}
func (m *Model) runOBS(kind string, fn func(context.Context) error) tea.Cmd {
	if m.obsBusy || m.busy {
		return nil
	}
	m.obsBusy = true
	m.obsOperation++
	id := m.obsOperation
	ctx, cancel := context.WithTimeout(m.ctx, obsOperationTimeout)
	m.obsCancel = cancel
	m.status = fmt.Sprintf(i18n.T(i18n.TUIStatusOperationPending), operationName(kind))
	return func() tea.Msg {
		defer cancel()
		if err := m.session.Lock(ctx); err != nil {
			return obsResultMsg{id: id, kind: kind, err: err}
		}
		defer m.session.Unlock()
		return obsResultMsg{id: id, kind: kind, err: fn(ctx)}
	}
}
func (m *Model) handleOBSResult(msg obsResultMsg) tea.Cmd {
	if msg.id != m.obsOperation {
		return nil
	}
	m.obsBusy = false
	m.obsCancel = nil
	m.obsState = m.obsClient.Snapshot()
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			m.log(i18n.T(i18n.TUILogOBSCanceled))
		} else if msg.kind == "obs-connect" {
			m.log(msg.err.Error())
		} else {
			m.log(fmt.Sprintf(i18n.T(i18n.TUILogOperationFailed), operationName(msg.kind), msg.err.Error()))
		}
	} else if msg.kind == "obs-connect" && m.obsState.Connected {
		m.log(i18n.T(i18n.TUILogOBSConnected))
	} else if msg.kind == "obs-disconnect" {
		m.log(i18n.T(i18n.TUILogOBSDisconnected))
	}
	return nil
}
func (m *Model) connectOBS() tea.Cmd {
	if m.obsClient.Snapshot().Connected || m.obsBusy {
		return nil
	}
	endpoint := m.config.OBSURL
	return m.runOBS("obs-connect", func(ctx context.Context) error { return m.session.ConnectOBS(ctx, endpoint) })
}
func (m *Model) disconnectOBS() tea.Cmd {
	return m.runOBS("obs-disconnect", func(context.Context) error { return m.obsClient.Disconnect() })
}
func (m *Model) startPrompt() string {
	text := i18n.T(i18n.TUIConfirmStartLive)
	if m.config.OBSAutoConnect {
		text += i18n.T(i18n.TUIConfirmStartAutoConnect)
	}
	if m.config.OBSAutoStream {
		return text + i18n.T(i18n.TUIConfirmStartAutoStream)
	}
	return text + i18n.T(i18n.TUIConfirmStartConfigureOnly)
}
func (m *Model) stopPrompt() string {
	state := m.obsClient.Snapshot()
	if state.Connected || m.config.OBSAutoStream || state.Status.Active || state.Status.Reconnecting {
		return i18n.T(i18n.TUIConfirmStopAutoStream)
	}
	return i18n.T(i18n.TUIConfirmStopManualStream)
}

func (m *Model) startLive() tea.Cmd {
	if !m.requireRoom() || m.obsBusy {
		return nil
	}
	room, cfg, client := *m.room, m.config, m.client
	return m.work("start", func(ctx context.Context) (any, error) { return m.session.Start(ctx, client, room, cfg) })
}
func (m *Model) stopLive() tea.Cmd {
	if !m.requireRoom() || m.obsBusy {
		return nil
	}
	room, cfg, client := *m.room, m.config, m.client
	return m.work("stop", func(ctx context.Context) (any, error) { return m.session.Stop(ctx, client, room, cfg) })
}
