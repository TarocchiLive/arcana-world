package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"arcana-world/internal/domain"
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
		return "[开] " + label
	}
	return "[关] " + label
}

// Close releases the OBS session and flushes the session journal exactly once.
func (m *Model) Close() error {
	m.closeOnce.Do(func() {
		if m.cancel != nil {
			m.cancel()
		}
		if m.obsCancel != nil {
			m.obsCancel()
		}
		m.closeErr = errors.Join(m.obsClient.Close(), m.journal.Write("应用退出"), m.journal.Close())
	})
	return m.closeErr
}

// Exactly one command consumes connection events; Init is guarded against re-entry.
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
	// A queued event may belong to a disconnected generation. Never publish its old state.
	m.obsState = m.obsClient.Snapshot()
	if previous.Connected && !m.obsState.Connected && m.obsState.Err != nil {
		m.log("OBS 连接已断开：" + m.obsState.Err.Error() + "；可手动重新连接")
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
	m.status = "正在" + operationName(kind) + "…"
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
			m.log("OBS 操作已取消")
		} else {
			m.log(operationName(msg.kind) + "失败：" + msg.err.Error())
		}
	} else if msg.kind == "obs-connect" && m.obsState.Connected {
		m.log("OBS 已连接；连接将保持到主动断开或退出")
	} else if msg.kind == "obs-disconnect" {
		m.log("OBS 控制连接已断开；未停止推流")
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
	text := "确定在 B 站开播？"
	if m.config.OBSAutoConnect {
		text += "未连接时会先尝试连接 OBS。"
	}
	if m.config.OBSAutoStream {
		return text + "将自动写入推流配置并启动 OBS 推流。"
	}
	return text + "连接 OBS 时自动写入配置，但不自动启动推流。"
}
func (m *Model) stopPrompt() string {
	if m.config.OBSAutoStream {
		return "确定停播？会先停止已连接 OBS 的推流，再关闭 B 站直播间。"
	}
	return "确定关闭 B 站直播间？自动推流未开启，请自行停止 OBS 推流。"
}
func (m *Model) startLive() tea.Cmd {
	if !m.requireRoom() || m.obsBusy {
		return nil
	}
	room, cfg := *m.room, m.config
	return m.work("start", func(ctx context.Context) (any, error) {
		if cfg.OBSAutoConnect && !m.obsClient.Snapshot().Connected {
			if err := m.connectSession(ctx, cfg.OBSURL); err != nil {
				return nil, fmt.Errorf("自动连接 OBS 失败，尚未开播：%w", err)
			}
		}
		connected := m.obsClient.Snapshot().Connected
		if cfg.OBSAutoStream && !connected {
			return nil, errors.New("自动推流已开启，请先连接 OBS 或开启自动连接")
		}
		if connected {
			status, err := m.obsClient.Status(ctx)
			if err != nil {
				return nil, fmt.Errorf("无法确认 OBS 状态，尚未开播：%w", err)
			}
			if status.Active || status.Reconnecting {
				return nil, errors.New("OBS 正在推流，不能覆盖当前推流目标；请先停播或断开 OBS 控制连接")
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
					return nil, fmt.Errorf("OBS 状态查询失败，尚未关闭直播间：%w", err)
				}
				if status.Active || status.Reconnecting {
					if err = m.obsClient.Stop(ctx); err != nil {
						return nil, fmt.Errorf("OBS 停止推流失败，尚未关闭直播间：%w", err)
					}
				}
				outcome.obsStopped = true
			} else {
				outcome.warning = "OBS 未连接，请确认 OBS 推流已手动停止"
			}
		} else {
			outcome.warning = "自动推流未开启，请在 OBS 中自行停止推流"
		}
		if err := m.client.Stop(ctx, roomID); err != nil {
			if outcome.obsStopped {
				return nil, fmt.Errorf("OBS 已停止推流，但 B 站关播失败：%w", err)
			}
			return nil, err
		}
		return outcome, nil
	})
}
