package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"arcana-world/internal/bili"
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
		m.obsClosing.Store(true)
		if m.cancel != nil {
			m.cancel()
		}
		if m.obsCancel != nil {
			m.obsCancel()
		}
		gateCtx, gateCancel := context.WithTimeout(context.Background(), 15*time.Second)
		var stopErr error
		select {
		case m.obsLifecycle <- struct{}{}:
			defer func() { <-m.obsLifecycle }()
		case <-gateCtx.Done():
			stopErr = gateCtx.Err()
		}
		gateCancel()
		cfg := m.store.Config()
		if !cfg.ExitOBSStopDisabled && stopErr == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			_, stopErr = m.stopControlledOBS(ctx, cfg, m.room != nil && m.room.Live)
			cancel()
		}
		if !cfg.ExitLiveStopDisabled {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			stopErr = errors.Join(stopErr, m.stopExitLive(ctx, cfg))
			cancel()
		}
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

func (m *Model) stopExitLive(ctx context.Context, cfg domain.Config) error {
	if cfg.ActiveUID == "" {
		return nil
	}
	account, err := m.store.Load(cfg.ActiveUID)
	if err != nil {
		return fmt.Errorf(i18n.T(i18n.TUIErrorExitLiveAccount), err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// 独立客户端不读取旧房间，也不修改尚未完成的界面请求的房间缓存。
	client, err := bili.New(cfg.Proxy)
	if err != nil {
		return fmt.Errorf(i18n.T(i18n.TUIErrorExitLiveRoom), err)
	}
	defer client.HTTP.CloseIdleConnections()
	client.APIBase, client.LiveBase, client.PassportBase = m.client.APIBase, m.client.LiveBase, m.client.PassportBase
	client.SetAccount(account)
	room, err := client.Room(ctx)
	if err != nil {
		return fmt.Errorf(i18n.T(i18n.TUIErrorExitLiveRoom), err)
	}
	if room.Live {
		if err := client.Stop(ctx, room.ID); err != nil {
			return fmt.Errorf(i18n.T(i18n.TUIErrorExitLiveStop), err)
		}
	}
	return nil
}

// 串行化完整的直播操作，避免退出时停止推流后又执行排队的启动。
func (m *Model) acquireOBS(ctx context.Context) error {
	if m.obsClosing.Load() {
		return context.Canceled
	}
	select {
	case m.obsLifecycle <- struct{}{}:
		if m.obsClosing.Load() || ctx.Err() != nil {
			<-m.obsLifecycle
			return context.Canceled
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
	return func() tea.Msg {
		defer cancel()
		if err := m.acquireOBS(ctx); err != nil {
			return obsResultMsg{id: id, kind: kind, err: err}
		}
		defer func() { <-m.obsLifecycle }()
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
	state := m.obsClient.Snapshot()
	if state.Connected || m.config.OBSAutoStream || state.Status.Active || state.Status.Reconnecting {
		return i18n.T(i18n.TUIConfirmStopAutoStream)
	}
	return i18n.T(i18n.TUIConfirmStopManualStream)
}

// 调用方持有 obsLifecycle；断线时使用既有凭据恢复会话，而非假定推流已停止。
func (m *Model) stopControlledOBS(ctx context.Context, cfg domain.Config, live bool) (bool, error) {
	state := m.obsClient.Snapshot()
	if !state.Connected {
		// 自动推流是偏好，不是正在推流的证据；已知活动状态在断线后仍需清理。
		if !(cfg.OBSAutoStream && live) && !state.Status.Active && !state.Status.Reconnecting {
			return false, nil
		}
		if err := m.connectSession(ctx, cfg.OBSURL); err != nil {
			return false, errors.New(i18n.T(i18n.TUIErrorOBSStopConnect))
		}
	}
	status, err := m.obsClient.Status(ctx)
	if err != nil {
		return false, fmt.Errorf(i18n.T(i18n.TUIErrorOBSStopStatus), err)
	}
	if status.Active || status.Reconnecting {
		if err := m.obsClient.Stop(ctx); err != nil {
			return false, fmt.Errorf(i18n.T(i18n.TUIErrorOBSStopStream), err)
		}
	}
	return true, nil
}
func (m *Model) startLive() tea.Cmd {
	if !m.requireRoom() || m.obsBusy {
		return nil
	}
	room, cfg := *m.room, m.config
	return m.work("start", func(ctx context.Context) (any, error) {
		if err := m.acquireOBS(ctx); err != nil {
			return nil, err
		}
		defer func() { <-m.obsLifecycle }()
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
	room, cfg := *m.room, m.config
	return m.work("stop", func(ctx context.Context) (any, error) {
		if err := m.acquireOBS(ctx); err != nil {
			return nil, err
		}
		defer func() { <-m.obsLifecycle }()
		stopped, err := m.stopControlledOBS(ctx, cfg, room.Live)
		if err != nil {
			return nil, err
		}
		outcome := stopOutcome{obsStopped: stopped}
		if err := m.client.Stop(ctx, room.ID); err != nil {
			if outcome.obsStopped {
				return nil, fmt.Errorf(i18n.T(i18n.TUIErrorLiveStopAfterOBS), err)
			}
			return nil, err
		}
		return outcome, nil
	})
}
