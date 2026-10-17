package app

import (
	"arcana-world/internal/bili"
	"arcana-world/internal/config"
	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"arcana-world/internal/journal"
	"arcana-world/internal/obs"
	"arcana-world/internal/store"
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zalando/go-keyring"
)

// 每个关闭阶段都有独立的截止时间，避免某一阶段失败
// 耗尽其余清理阶段可用的时间。
const shutdownPhaseTimeout = 15 * time.Second

// Session 负责串行执行身份切换与直播状态转换，包括关闭流程。
// UI 负责显示快照；命令返回已确认的远端操作结果。
type Session struct {
	store     *store.Store
	obsClient *obs.Client
	gate      chan struct{}
	closing   atomic.Bool
	clientsMu sync.Mutex
	clients   map[*bili.Client]struct{}
}

var logger = journal.Logger("app")

func New(s *store.Store, o *obs.Client, c *bili.Client) *Session {
	return &Session{store: s, obsClient: o, gate: make(chan struct{}, 1), clients: map[*bili.Client]struct{}{c: {}}}
}
func (m *Session) Track(c *bili.Client) {
	m.clientsMu.Lock()
	defer m.clientsMu.Unlock()
	if m.closing.Load() {
		c.HTTP.CloseIdleConnections()
		return
	}
	m.clients[c] = struct{}{}
}
func (m *Session) Release(c *bili.Client) {
	if c == nil {
		return
	}
	c.HTTP.CloseIdleConnections()
	m.clientsMu.Lock()
	delete(m.clients, c)
	m.clientsMu.Unlock()
}
func (m *Session) BeginClose() { m.closing.Store(true) }
func (m *Session) Unlock()     { <-m.gate }
func (m *Session) Lock(ctx context.Context) error {
	if m.closing.Load() {
		return context.Canceled
	}
	select {
	case m.gate <- struct{}{}:
		if m.closing.Load() || ctx.Err() != nil {
			<-m.gate
			return context.Canceled
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (m *Session) ConnectOBS(ctx context.Context, endpoint string) error {
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

// 调用方持有 session gate；断线时使用既有凭据恢复会话，而非假定推流已停止。
func (m *Session) stopControlledOBS(ctx context.Context, cfg config.Config, live bool) (bool, error) {
	state := m.obsClient.Snapshot()
	if !state.Connected {
		// 自动推流是偏好，不是正在推流的证据；已知活动状态在断线后仍需清理。
		if !(cfg.OBSAutoStream && live) && !state.Status.Active && !state.Status.Reconnecting {
			return false, nil
		}
		if err := m.ConnectOBS(ctx, cfg.OBSURL); err != nil {
			logger.Error("obs reconnect for stop failed", "endpoint", cfg.OBSURL, "err", err)
			return false, fmt.Errorf("%s: %w", i18n.T(i18n.TUIErrorOBSStopConnect), err)
		}
	}
	status, err := m.obsClient.Status(ctx)
	if err != nil {
		logger.Error("obs status query failed before stop", "err", err)
		return false, fmt.Errorf(i18n.T(i18n.TUIErrorOBSStopStatus), err)
	}
	if status.Active || status.Reconnecting {
		if err := m.obsClient.Stop(ctx); err != nil {
			logger.Error("obs stop stream failed", "err", err)
			return false, fmt.Errorf(i18n.T(i18n.TUIErrorOBSStopStream), err)
		}
		logger.Info("obs stream stopped")
	}
	return true, nil
}

type StartOutcome struct {
	Stream                    domain.Stream
	OBSConfigured, OBSStarted bool
	OBSErr                    error
}
type StopOutcome struct{ OBSStopped bool }

func (m *Session) Start(ctx context.Context, client *bili.Client, room domain.Room, cfg config.Config) (StartOutcome, error) {
	if err := m.Lock(ctx); err != nil {
		return StartOutcome{}, err
	}
	defer m.Unlock()
	logger.Debug("live start begin", "room", room.ID, "area", room.AreaID, "protocol", cfg.Protocol, "obs_auto_connect", cfg.OBSAutoConnect, "obs_auto_stream", cfg.OBSAutoStream, "obs_connected", m.obsClient.Snapshot().Connected)
	if cfg.OBSAutoConnect && !m.obsClient.Snapshot().Connected {
		if err := m.ConnectOBS(ctx, cfg.OBSURL); err != nil {
			logger.Error("obs auto connect failed before live start", "room", room.ID, "endpoint", cfg.OBSURL, "err", err)
			return StartOutcome{}, fmt.Errorf(i18n.T(i18n.TUIErrorOBSAutoConnect), err)
		}
	}
	connected := m.obsClient.Snapshot().Connected
	if cfg.OBSAutoStream && !connected {
		logger.Error("live start rejected, obs not connected", "room", room.ID, "endpoint", cfg.OBSURL)
		return StartOutcome{}, errors.New(i18n.T(i18n.TUIErrorOBSConnectionRequired))
	}
	if connected {
		status, err := m.obsClient.Status(ctx)
		if err != nil {
			logger.Error("obs status query failed before live start", "room", room.ID, "err", err)
			return StartOutcome{}, fmt.Errorf(i18n.T(i18n.TUIErrorOBSStartStatus), err)
		}
		if status.Active || status.Reconnecting {
			logger.Error("live start rejected, obs stream already active", "room", room.ID, "active", status.Active, "reconnecting", status.Reconnecting)
			return StartOutcome{}, errors.New(i18n.T(i18n.TUIErrorOBSDestinationActive))
		}
	}
	stream, err := client.Start(ctx, room.ID, room.AreaID, cfg.Protocol)
	if err != nil {
		logger.Error("live start request failed", "room", room.ID, "area", room.AreaID, "protocol", cfg.Protocol, "err", err)
		return StartOutcome{}, err
	}
	outcome := StartOutcome{Stream: stream}
	if !connected {
		logger.Info("live started", "room", room.ID, "area", room.AreaID, "protocol", cfg.Protocol, "obs_connected", false)
		return outcome, nil
	}
	if err = m.obsClient.Configure(ctx, stream); err != nil {
		logger.Warn("obs configure failed after live start", "room", room.ID, "err", err)
		outcome.OBSErr = err
		logger.Info("live started", "room", room.ID, "area", room.AreaID, "protocol", cfg.Protocol, "obs_configured", false, "obs_started", false)
		return outcome, nil
	}
	outcome.OBSConfigured = true
	if cfg.OBSAutoStream {
		if err = m.obsClient.Start(ctx); err != nil {
			logger.Warn("obs start stream failed after live start", "room", room.ID, "err", err)
			outcome.OBSErr = err
		} else {
			outcome.OBSStarted = true
		}
	}
	logger.Info("live started", "room", room.ID, "area", room.AreaID, "protocol", cfg.Protocol, "obs_configured", true, "obs_started", outcome.OBSStarted)
	return outcome, nil
}
func (m *Session) Stop(ctx context.Context, client *bili.Client, room domain.Room, cfg config.Config) (StopOutcome, error) {
	if err := m.Lock(ctx); err != nil {
		return StopOutcome{}, err
	}
	defer m.Unlock()
	logger.Debug("live stop begin", "room", room.ID, "obs_auto_stream", cfg.OBSAutoStream)
	stopped, err := m.stopControlledOBS(ctx, cfg, room.Live)
	if err != nil {
		return StopOutcome{}, err
	}
	outcome := StopOutcome{OBSStopped: stopped}
	if err := client.Stop(ctx, room.ID); err != nil {
		logger.Error("live stop failed", "room", room.ID, "obs_stopped", outcome.OBSStopped, "err", err)
		if outcome.OBSStopped {
			return outcome, fmt.Errorf(i18n.T(i18n.TUIErrorLiveStopAfterOBS), err)
		}
		return outcome, err
	}
	logger.Info("live stopped", "room", room.ID, "obs_stopped", outcome.OBSStopped)
	return outcome, nil
}
func (m *Session) stopExitLive(ctx context.Context, cfg config.Config, source *bili.Client) error {
	if cfg.ActiveUID == "" {
		return nil
	}
	account, err := m.store.Load(cfg.ActiveUID)
	if err != nil {
		logger.Error("exit live account load failed", "uid", cfg.ActiveUID, "err", err)
		return fmt.Errorf(i18n.T(i18n.TUIErrorExitLiveAccount), err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// 独立客户端不读取旧房间，也不修改尚未完成的界面请求的房间缓存。
	client, err := m.client(source, &account, cfg.Proxy)
	if err != nil {
		logger.Error("exit live client init failed", "uid", account.UID, "err", err)
		return fmt.Errorf(i18n.T(i18n.TUIErrorExitLiveRoom), err)
	}
	defer client.HTTP.CloseIdleConnections()
	room, err := client.Room(ctx)
	if err != nil {
		logger.Error("exit live room query failed", "uid", account.UID, "err", err)
		return fmt.Errorf(i18n.T(i18n.TUIErrorExitLiveRoom), err)
	}
	if room.Live {
		if err := client.Stop(ctx, room.ID); err != nil {
			logger.Error("exit live stop failed", "room", room.ID, "err", err)
			return fmt.Errorf(i18n.T(i18n.TUIErrorExitLiveStop), err)
		}
		logger.Info("live stopped on exit", "room", room.ID)
	}
	return nil
}

func (m *Session) Close(source *bili.Client, live bool) error {
	m.BeginClose()
	defer func() {
		m.clientsMu.Lock()
		for c := range m.clients {
			c.HTTP.CloseIdleConnections()
			delete(m.clients, c)
		}
		m.clientsMu.Unlock()
		source.HTTP.CloseIdleConnections()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), shutdownPhaseTimeout)
	defer cancel()
	var gateErr error
	select {
	case m.gate <- struct{}{}:
		defer m.Unlock()
	case <-ctx.Done():
		gateErr = ctx.Err()
	}
	cfg := m.store.Config()
	err := gateErr
	if !cfg.ExitOBSStopDisabled && gateErr == nil {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownPhaseTimeout)
		_, err = m.stopControlledOBS(ctx, cfg, live)
		cancel()
	}
	if !cfg.ExitLiveStopDisabled {
		ctx, cancel := context.WithTimeout(context.Background(), shutdownPhaseTimeout)
		err = errors.Join(err, m.stopExitLive(ctx, cfg, source))
		cancel()
	}
	return err
}
