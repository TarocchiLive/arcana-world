package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"

	"arcana-world/internal/broadcast"
	"arcana-world/internal/danmaku"
	"arcana-world/internal/i18n"
	"arcana-world/internal/presentation"
	tea "charm.land/bubbletea/v2"
)

// ConfigureWebSocket 在界面启动前应用保存的开关；命令行地址仅本次生效。
// 绑定失败只提示，不阻止程序启动。
func (m *Model) ConfigureWebSocket(addr string) error {
	if m.initialized {
		return errors.New("websocket must be configured before model initialization")
	}
	if addr != "" && !validWebSocketAddr(addr) {
		return errors.New(i18n.T(i18n.TUISettingsWebSocketAddrInvalid))
	}
	m.websocketAddr = addr
	if m.config.WebSocketEnabled || addr != "" {
		if err := m.startWebSocket(); err != nil {
			logger.Warn("websocket broadcast start failed", "addr", m.effectiveWebSocketAddr(), "err", err)
			m.warn(fmt.Sprintf(i18n.T(i18n.TUISettingsWebSocketStartFailed), err))
		}
	}
	return nil
}

// validWebSocketAddr 只接受 IP 或 localhost 加端口：空主机会绑定所有网卡，主机名需要 DNS 查询。
func validWebSocketAddr(addr string) bool {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip, err := netip.ParseAddr(host)
	return err == nil && ip.Zone() == ""
}

func (m *Model) startWebSocket() error {
	if m.websocket != nil {
		return nil
	}
	if m.chat == nil || m.chat.listener == nil {
		if m.chat != nil && m.chat.err != nil {
			return m.chat.err
		}
		return errors.New("danmaku listener unavailable")
	}
	token, err := m.store.WebSocketToken()
	if err != nil {
		return err
	}
	server, err := broadcast.Listen(m.effectiveWebSocketAddr(), token)
	if err != nil {
		return err
	}
	m.websocket = server
	m.setChatBroadcast()
	return nil
}

func (m *Model) closeWebSocket() error {
	if m.websocket == nil {
		return nil
	}
	server := m.websocket
	m.websocket = nil
	m.setChatBroadcast()
	return server.Close()
}

// setChatBroadcast 按当前服务和全局屏蔽词重建监听器的广播接收器。
func (m *Model) setChatBroadcast() {
	if m.chat == nil || m.chat.listener == nil {
		return
	}
	server := m.websocket
	if server == nil {
		m.chat.listener.SetBroadcast(nil)
		return
	}
	blocked := m.config.Blocklist.DisplayMatcher()
	m.chat.listener.SetBroadcast(func(room int64, raw json.RawMessage) {
		// 无人连接时跳过解析。
		if !server.Connected() {
			return
		}
		events := danmaku.Project(room, raw)
		forward := false
		for _, e := range events {
			if blocked.Blocks(e) {
				return
			}
			forward = forward || broadcastEvent(e)
		}
		if forward {
			server.Publish(room, raw)
		}
	})
}

// broadcastEvent 沿用聊天页默认显示的事件，并补充分享和关注类互动。
func broadcastEvent(e danmaku.Event) bool {
	switch e.Kind {
	case "share", "special_follow", "mutual_follow":
		return true
	}
	return presentation.Enabled(nil, e)
}

func (m *Model) toggleWebSocket() tea.Cmd {
	cfg := m.config
	cfg.WebSocketEnabled = m.websocket == nil
	if cfg.WebSocketEnabled {
		if err := m.startWebSocket(); err != nil {
			m.warn(fmt.Sprintf(i18n.T(i18n.TUISettingsWebSocketStartFailed), err))
			return nil
		}
	}
	if err := m.store.SaveConfig(cfg); err != nil {
		if cfg.WebSocketEnabled {
			err = errors.Join(err, m.closeWebSocket())
		}
		m.warn(err.Error())
		return nil
	}
	m.config = m.store.Config()
	if !cfg.WebSocketEnabled {
		if err := m.closeWebSocket(); err != nil {
			m.warn(err.Error())
			return nil
		}
	}
	m.log(i18n.T(i18n.TUILogSettingsSaved))
	return m.finishResult()
}

func (m *Model) effectiveWebSocketAddr() string {
	if m.websocketAddr != "" {
		return m.websocketAddr
	}
	return m.config.WebSocketAddr
}

func (m *Model) saveWebSocketAddr(addr string) tea.Cmd {
	if m.websocketAddr != "" {
		m.warn(i18n.T(i18n.TUISessionOverride))
		return nil
	}
	if !validWebSocketAddr(addr) {
		m.warnStatus(i18n.T(i18n.TUISettingsWebSocketAddrInvalid))
		return nil
	}
	var replacement *broadcast.Server
	if m.websocket != nil && addr != m.config.WebSocketAddr {
		token, err := m.store.WebSocketToken()
		if err == nil {
			replacement, err = broadcast.Listen(addr, token)
		}
		if err != nil {
			m.warnStatus(clean(err.Error()))
			return nil
		}
	}
	cfg := m.config
	cfg.WebSocketAddr = addr
	if err := m.store.SaveConfig(cfg); err != nil {
		if replacement != nil {
			err = errors.Join(err, replacement.Close())
		}
		m.warnStatus(clean(err.Error()))
		return nil
	}
	m.config = m.store.Config()
	if replacement != nil {
		old := m.websocket
		m.websocket = replacement
		m.setChatBroadcast()
		if err := old.Close(); err != nil {
			m.warn(err.Error())
		}
	}
	m.mode = ""
	m.input.SetValue("")
	m.input.Blur()
	m.log(i18n.T(i18n.TUILogSettingsSaved))
	return m.finishResult()
}

var websocketTokenOperation = operation[string]{label: i18n.TUIOperationSaveSettings, handle: (*Model).handleWebSocketTokenResult}

// saveWebSocketToken 在后台写入凭据存储；空令牌表示关闭验证。
func (m *Model) saveWebSocketToken(token string) tea.Cmd {
	return work(m, websocketTokenOperation, func(context.Context) (string, error) {
		return token, m.store.SetWebSocketToken(token)
	})
}

func (m *Model) handleWebSocketTokenResult(token string, err error, label i18n.Key) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	if m.websocket != nil {
		m.websocket.SetToken(token)
	}
	m.log(i18n.T(i18n.TUILogSettingsSaved))
	return m.finishResult()
}
