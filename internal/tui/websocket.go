package tui

import (
	"errors"
	"net"
	"strconv"

	"arcana-world/internal/broadcast"
	"arcana-world/internal/i18n"
	"arcana-world/internal/webui"
	tea "charm.land/bubbletea/v2"
)

// ConfigureWebSocket 在界面启动前应用保存的开关；命令行地址仅本次生效。
func (m *Model) ConfigureWebSocket(addr string, webServer *webui.Server) error {
	if m.initialized {
		return errors.New("websocket must be configured before model initialization")
	}
	m.websocketAddr = addr
	m.webServer = webServer
	if m.config.WebSocketEnabled || addr != "" {
		return m.startWebSocket()
	}
	return nil
}

func (m *Model) startWebSocket() error {
	if m.websocket != nil {
		return nil
	}
	addr := m.effectiveWebSocketAddr()
	server, err := m.newWebSocket(addr)
	if err != nil {
		return err
	}
	if err := m.setChatBroadcast(server.Publish); err != nil {
		return errors.Join(err, server.Close())
	}
	m.websocket = server
	m.attachWebSocket()
	return nil
}

// 地址完全一致时共用 WebUI 监听器，不扩大任一服务的监听范围。
func (m *Model) newWebSocket(addr string) (*broadcast.Server, error) {
	if m.webServer != nil && addr == m.webServer.Addr() {
		return broadcast.New(), nil
	}
	return broadcast.Listen(addr)
}

func (m *Model) attachWebSocket() {
	if m.webServer == nil {
		return
	}
	var shared *broadcast.Server
	if m.effectiveWebSocketAddr() == m.webServer.Addr() {
		shared = m.websocket
	}
	m.webServer.SetBroadcast(shared)
}

func (m *Model) closeWebSocket() error {
	if m.websocket == nil {
		return nil
	}
	m.chat.listener.SetBroadcast(nil)
	server := m.websocket
	m.websocket = nil
	m.attachWebSocket()
	return server.Close()
}

func (m *Model) toggleWebSocket() tea.Cmd {
	cfg := m.config
	cfg.WebSocketEnabled = m.websocket == nil
	if cfg.WebSocketEnabled {
		if err := m.startWebSocket(); err != nil {
			m.warn(err.Error())
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
	_, port, err := net.SplitHostPort(addr)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 1 || n > 65535 {
		m.warnStatus(i18n.T(i18n.TUIWebSocketAddrInvalid))
		return nil
	}
	if _, err := net.ResolveTCPAddr("tcp", addr); err != nil {
		m.warnStatus(err.Error())
		return nil
	}
	var replacement *broadcast.Server
	if m.websocket != nil && addr != m.config.WebSocketAddr {
		var err error
		replacement, err = m.newWebSocket(addr)
		if err != nil {
			m.warnStatus(err.Error())
			return nil
		}
	}
	cfg := m.config
	cfg.WebSocketAddr = addr
	if err := m.store.SaveConfig(cfg); err != nil {
		if replacement != nil {
			err = errors.Join(err, replacement.Close())
		}
		m.warnStatus(err.Error())
		return nil
	}
	m.config = m.store.Config()
	if replacement != nil {
		old := m.websocket
		m.websocket = replacement
		m.chat.listener.SetBroadcast(replacement.Publish)
		m.attachWebSocket()
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
