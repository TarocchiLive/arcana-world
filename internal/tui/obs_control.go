package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"arcana-world/internal/app"
	"arcana-world/internal/i18n"
	"arcana-world/internal/termimage"
	tea "charm.land/bubbletea/v2"
)

const obsOperationTimeout = 30 * time.Second

type obsEventMsg struct{ closed bool }
type obsResultMsg struct {
	id      int
	label   i18n.Key
	err     error
	started time.Time // 任务开始执行的时间，用于计算耗时
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
		// 事件循环可能因信号直接结束，此时不能再依靠 tea.Cmd 清理终端。
		imageID := m.inlineImage.id
		m.clearInlineImage()
		var imageErr error
		if imageID != 0 {
			_, imageErr = fmt.Fprint(os.Stdout, termimage.Delete(imageID))
		}
		if m.speaker != nil && m.speaker.cancel != nil {
			m.speaker.cancel()
		}
		m.session.BeginClose()
		if m.cancel != nil {
			m.cancel()
		}
		if m.obsCancel != nil {
			m.obsCancel()
		}
		ttsErr := m.closeTTS()
		stopErr := m.session.Close(m.client, m.room != nil && m.room.Live)
		if stopErr != nil {
			logger.Error("close live session failed", "err", stopErr)
		}
		closeErr := errors.Join(imageErr, ttsErr, stopErr, m.closeOverlay(), m.closeChat(), m.obsClient.Close())
		// 退出记录必须在日志关闭之前写入，并带上前面各步骤的错误。
		logger.Info("application exited", "err", closeErr, "clear_data", m.clearDataOnExit)
		m.closeErr = errors.Join(closeErr, m.journal.Close())
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
	if previous.Connected && !m.obsState.Connected {
		if m.obsState.Err != nil {
			logger.Warn("obs connection lost", "closed", msg.closed, "err", m.obsState.Err)
		} else {
			logger.Info("obs disconnected", "closed", msg.closed)
		}
	}
	if previous.Connected && !m.obsState.Connected && m.obsState.Err != nil {
		m.warn(fmt.Sprintf(i18n.T(i18n.TUILogOBSConnectionLost), m.obsState.Err.Error()))
	}
	if m.obsState.Connected && m.obsState.Err != nil &&
		(previous.Err == nil || previous.Err.Error() != m.obsState.Err.Error()) {
		logger.Warn("obs stream transport failed", "err", m.obsState.Err)
		m.warn(m.obsState.Err.Error())
	}
	if msg.closed {
		return nil
	}
	return m.watchOBS()
}
func (m *Model) runOBS(label i18n.Key, fn func(context.Context) error) tea.Cmd {
	if m.obsBusy || m.busy {
		return nil
	}
	m.obsBusy = true
	m.obsOperation++
	id := m.obsOperation
	ctx, cancel := context.WithTimeout(m.ctx, obsOperationTimeout)
	m.obsCancel = cancel
	m.progressStatus(fmt.Sprintf(i18n.T(i18n.TUIStatusOperationPending), i18n.T(label)))
	logger.Debug("operation started", "op", string(label), "id", id)
	return func() tea.Msg {
		defer cancel()
		started := time.Now()
		if err := m.session.Lock(ctx); err != nil {
			return obsResultMsg{id: id, label: label, err: err, started: started}
		}
		defer m.session.Unlock()
		return obsResultMsg{id: id, label: label, err: fn(ctx), started: started}
	}
}
func (m *Model) handleOBSResult(msg obsResultMsg) tea.Cmd {
	if msg.id != m.obsOperation {
		return nil
	}
	m.obsBusy = false
	m.obsCancel = nil
	m.obsState = m.obsClient.Snapshot()
	elapsed := time.Duration(0)
	if !msg.started.IsZero() {
		elapsed = time.Since(msg.started)
	}
	op := string(msg.label)
	switch {
	case msg.err == nil:
		logger.Info("operation succeeded", "op", op, "id", msg.id, "elapsed", elapsed, "obs_connected", m.obsState.Connected)
	case errors.Is(msg.err, context.Canceled):
		logger.Warn("operation canceled", "op", op, "id", msg.id, "elapsed", elapsed, "err", msg.err)
	default:
		logger.Error("operation failed", "op", op, "id", msg.id, "elapsed", elapsed, "err", msg.err)
	}
	if msg.err != nil {
		if errors.Is(msg.err, context.Canceled) {
			m.log(i18n.T(i18n.TUILogOBSCanceled))
		} else if msg.label == i18n.TUIOBSConnect {
			m.warn(msg.err.Error())
		} else {
			m.warn(fmt.Sprintf(i18n.T(i18n.TUILogOperationFailed), i18n.T(msg.label), msg.err.Error()))
		}
	} else if msg.label == i18n.TUIOBSConnect && m.obsState.Connected {
		m.log(i18n.T(i18n.TUILogOBSConnected))
	} else if msg.label == i18n.TUIOBSDisconnect {
		m.log(i18n.T(i18n.TUILogOBSDisconnected))
	}
	return nil
}
func (m *Model) connectOBS() tea.Cmd {
	if m.obsClient.Snapshot().Connected || m.obsBusy {
		return nil
	}
	endpoint := m.config.OBSURL
	return m.runOBS(i18n.TUIOBSConnect, func(ctx context.Context) error { return m.session.ConnectOBS(ctx, endpoint) })
}
func (m *Model) disconnectOBS() tea.Cmd {
	return m.runOBS(i18n.TUIOBSDisconnect, func(context.Context) error { return m.obsClient.Disconnect() })
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
	return work(m, startOperation(), func(ctx context.Context) (app.StartOutcome, error) { return m.session.Start(ctx, client, room, cfg) })
}
func (m *Model) stopLive() tea.Cmd {
	if !m.requireRoom() || m.obsBusy {
		return nil
	}
	room, cfg, client := *m.room, m.config, m.client
	return work(m, stopOperation(), func(ctx context.Context) (app.StopOutcome, error) { return m.session.Stop(ctx, client, room, cfg) })
}
