package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/zalando/go-keyring"
)

type obsEventMsg struct{ closed bool }
type obsResultMsg struct {
	id   int
	kind string
	err  error
}
type startOutcome struct {
	stream                    domain.Stream
	obsConfigured, obsStarted bool
	obsErr                    error
}
type stopOutcome struct {
	obsStopped bool
	warning    string
}

func toggleLabel(label string, enabled bool) string {
	if enabled {
		return i18n.T(i18n.TUIToggleOn) + label
	}
	return i18n.T(i18n.TUIToggleOff) + label
}

// Close 仅释放一次 OBS 与浮层会话，并刷新会话日志。
func (m *Model) Close() error {
	m.closeOnce.Do(func() {
		if m.cancel != nil {
			m.cancel()
		}
		if m.obsCancel != nil {
			m.obsCancel()
		}
		m.closeErr = errors.Join(m.closeOverlay(), m.obsClient.Close(), m.journal.Write(i18n.T(i18n.TUILogApplicationExited)), m.journal.Close())
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
	ctx, cancel := context.WithTimeout(m.ctx, 30*time.Second)
	m.obsCancel = cancel
	m.status = fmt.Sprintf(i18n.T(i18n.TUIStatusOperationPending), operationName(kind))
	return func() tea.Msg { defer cancel(); return obsResultMsg{id: id, kind: kind, err: fn(ctx)} }
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
func (m *Model) connectSession(ctx context.Context, endpoint string) error {
	password, err := m.store.OBSSecret()
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	if err := m.obsClient.Connect(ctx, endpoint, password); err != nil {
		return err
	}
	_, err = m.obsClient.Status(ctx)
	return err
}
func (m *Model) connectOBS() tea.Cmd {
	if m.obsClient.Snapshot().Connected || m.obsBusy {
		return nil
	}
	endpoint := m.config.OBSURL
	return m.runOBS("obs-connect", func(ctx context.Context) error { return m.connectSession(ctx, endpoint) })
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
	if m.config.OBSAutoStream {
		return i18n.T(i18n.TUIConfirmStopAutoStream)
	}
	return i18n.T(i18n.TUIConfirmStopManualStream)
}
func (m *Model) startLive() tea.Cmd {
	if !m.requireRoom() || m.obsBusy {
		return nil
	}
	room, cfg := *m.room, m.config
	return m.work("start", func(ctx context.Context) (any, error) {
		if cfg.OBSAutoConnect && !m.obsClient.Snapshot().Connected {
			if err := m.connectSession(ctx, cfg.OBSURL); err != nil {
				return nil, fmt.Errorf(i18n.T(i18n.TUIErrorOBSAutoConnect), err)
			}
		}
		connected := m.obsClient.Snapshot().Connected
		if cfg.OBSAutoStream && !connected {
			return nil, errors.New(i18n.T(i18n.TUIErrorOBSConnectionRequired))
		}
		if connected {
			status, err := m.obsClient.Status(ctx)
			if err != nil {
				return nil, fmt.Errorf(i18n.T(i18n.TUIErrorOBSStartStatus), err)
			}
			if status.Active || status.Reconnecting {
				return nil, errors.New(i18n.T(i18n.TUIErrorOBSDestinationActive))
			}
		}
		stream, err := m.client.Start(ctx, room.ID, room.AreaID, cfg.Protocol)
		if err != nil {
			return nil, err
		}
		outcome := startOutcome{stream: stream}
		if !connected {
			return outcome, nil
		}
		if err = m.obsClient.Configure(ctx, stream); err != nil {
			outcome.obsErr = err
			return outcome, nil
		}
		outcome.obsConfigured = true
		if cfg.OBSAutoStream {
			if err = m.obsClient.Start(ctx); err != nil {
				outcome.obsErr = err
			} else {
				outcome.obsStarted = true
			}
		}
		return outcome, nil
	})
}
func (m *Model) stopLive() tea.Cmd {
	if !m.requireRoom() || m.obsBusy {
		return nil
	}
	roomID, cfg := m.room.ID, m.config
	return m.work("stop", func(ctx context.Context) (any, error) {
		outcome := stopOutcome{}
		if cfg.OBSAutoStream {
			if m.obsClient.Snapshot().Connected {
				status, err := m.obsClient.Status(ctx)
				if err != nil {
					return nil, fmt.Errorf(i18n.T(i18n.TUIErrorOBSStopStatus), err)
				}
				if status.Active || status.Reconnecting {
					if err = m.obsClient.Stop(ctx); err != nil {
						return nil, fmt.Errorf(i18n.T(i18n.TUIErrorOBSStopStream), err)
					}
				}
				outcome.obsStopped = true
			} else {
				outcome.warning = i18n.T(i18n.TUIWarningOBSDisconnectedStop)
			}
		} else {
			outcome.warning = i18n.T(i18n.TUIWarningOBSManualStop)
		}
		if err := m.client.Stop(ctx, roomID); err != nil {
			if outcome.obsStopped {
				return nil, fmt.Errorf(i18n.T(i18n.TUIErrorLiveStopAfterOBS), err)
			}
			return nil, err
		}
		return outcome, nil
	})
}
