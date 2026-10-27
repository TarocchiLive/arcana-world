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
	tea "charm.land/bubbletea/v2"
)

// ConfigureWebSocket 在界面启动前应用保存的开关；命令行地址仅本次生效。
// 绑定失败只提示，不阻止程序启动。
func (m *Model) ConfigureWebSocket(addr string) error {
	if m.initialized {
		return errors.New("websocket must be configured before model initialization")
	}
	m.websocketAddr = addr
	m.websocketSessionEnabled = addr != ""
	if m.webSocketEnabled() {
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
	token, err := m.store.WebSocketToken()
	m.cacheWebSocketTokenStatus(token != "", err)
	if err != nil {
		return err
	}
	server, err := m.listenWebSocket(m.effectiveWebSocketAddr(), token)
	if err != nil {
		return err
	}
	m.websocket = server
	m.setChatBroadcast()
	return nil
}

// listenWebSocket 统一校验最终地址，并避免 localhost 触发 DNS 查询。
func (m *Model) listenWebSocket(addr, token string) (*broadcast.Server, error) {
	if !validWebSocketAddr(addr) {
		return nil, errors.New(i18n.T(i18n.TUISettingsWebSocketAddrInvalid))
	}
	if m.chat == nil || m.chat.listener == nil {
		if m.chat != nil && m.chat.err != nil {
			return nil, m.chat.err
		}
		return nil, errors.New("danmaku listener unavailable")
	}
	host, port, _ := net.SplitHostPort(addr)
	if host == "localhost" {
		addr = net.JoinHostPort("127.0.0.1", port)
	}
	return broadcast.Listen(addr, token)
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
	m.chat.listener.SetBroadcast(func(room int64, raw json.RawMessage) func() {
		if !server.Connected() {
			return nil
		}
		forward := false
		for e := range danmaku.Project(room, raw) {
			if blocked.Blocks(e) {
				return nil
			}
			forward = forward || broadcastEvent(e)
		}
		if !forward {
			return nil
		}
		return server.Prepare(room, raw)
	})
}

// broadcastEvent 明确对外广播范围，不随界面事件目录的增减改变。
func broadcastEvent(e danmaku.Event) bool {
	switch e.Kind {
	case "chat", "gift", "sc", "guard", "delete",
		"enter", "follow", "like", "share", "special_follow", "mutual_follow",
		"live", "preparing", "room_change", "room_block", "cut_off":
		return true
	case "detail":
		switch e.Text {
		case "USER_TOAST_MSG", "GUIARD_MSG", "LIVE_OPEN_PLATFORM_SEND_GIFT",
			"LIVE_OPEN_PLATFORM_GUARD", "RECALL_DANMU_MSG", "WARNING",
			"ROOM_SILENT_ON", "ROOM_SILENT_OFF", "room_admin_entrance", "ROOM_ADMIN_REVOKE":
			return true
		}
	}
	return false
}

func (m *Model) webSocketEnabled() bool {
	if m.websocketAddr != "" {
		return m.websocketSessionEnabled
	}
	return m.config.WebSocketEnabled
}

func (m *Model) toggleWebSocket() tea.Cmd {
	return m.changeWebSocket(websocketChange{
		enabled: !m.webSocketEnabled(),
		addr:    m.effectiveWebSocketAddr(),
	})
}

func (m *Model) effectiveWebSocketAddr() string {
	if m.websocketAddr != "" {
		return m.websocketAddr
	}
	return m.config.WebSocketAddr
}

// webSocketTokenStatus 只读取缓存，渲染界面时不访问凭据存储。
func (m *Model) webSocketTokenStatus() string {
	if m.websocketTokenState == "" {
		return i18n.T(i18n.TUISettingsWebSocketTokenLoading)
	}
	return i18n.T(m.websocketTokenState)
}

func (m *Model) cacheWebSocketTokenStatus(set bool, err error) {
	switch {
	case err != nil:
		m.websocketTokenState = i18n.TUISettingsWebSocketTokenUnreadable
	case set:
		m.websocketTokenState = i18n.TUISettingsWebSocketTokenSet
	default:
		m.websocketTokenState = i18n.TUISettingsWebSocketTokenUnset
	}
}

var websocketTokenStatusOperation = operation[bool]{
	label: i18n.TUIOperationReadWebSocketToken,
	handle: func(m *Model, set bool, err error, label i18n.Key) tea.Cmd {
		m.cacheWebSocketTokenStatus(set, err)
		pending := fmt.Sprintf(i18n.T(i18n.TUIStatusOperationPending), i18n.T(label))
		if m.status == pending || (m.canceled && m.status == i18n.T(i18n.TUIStatusCancelRequested)) {
			// 只清除本次读取的进度，保留期间产生的其他提示与通知。
			m.progressStatus("")
		}
		// 仅更新状态，不重置详情页或正在编辑的输入。
		return nil
	},
}

// loadWebSocketTokenStatus 异步读取令牌，只将是否设置带回界面。
func (m *Model) loadWebSocketTokenStatus() tea.Cmd {
	cmd := work(m, websocketTokenStatusOperation, func(context.Context) (bool, error) {
		token, err := m.store.WebSocketToken()
		return token != "", err
	})
	if cmd != nil {
		m.websocketTokenState = i18n.TUISettingsWebSocketTokenLoading
	}
	return cmd
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
	return m.changeWebSocket(websocketChange{enabled: m.webSocketEnabled(), addr: addr, editingAddr: true})
}

type websocketChange struct {
	enabled     bool
	addr        string
	editingAddr bool
	token       string
}

var websocketChangeOperation = operation[websocketChange]{
	label:           i18n.TUIOperationSaveSettings,
	discardOnCancel: true,
	handle:          (*Model).handleWebSocketChange,
}

func (m *Model) changeWebSocket(change websocketChange) tea.Cmd {
	if change.enabled && !validWebSocketAddr(change.addr) {
		m.warnStatus(i18n.T(i18n.TUISettingsWebSocketAddrInvalid))
		return nil
	}
	if change.enabled && (m.websocket == nil || change.addr != m.effectiveWebSocketAddr()) {
		// 后台只读凭据；退出后丢弃结果不会遗留已监听的服务。
		return work(m, websocketChangeOperation, func(context.Context) (websocketChange, error) {
			token, err := m.store.WebSocketToken()
			change.token = token
			return change, err
		})
	}
	return m.handleWebSocketChange(change, nil, websocketChangeOperation.label)
}

func (m *Model) handleWebSocketChange(change websocketChange, err error, label i18n.Key) tea.Cmd {
	if change.enabled && (m.websocket == nil || change.addr != m.effectiveWebSocketAddr()) {
		m.cacheWebSocketTokenStatus(change.token != "", err)
	}
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	var replacement *broadcast.Server
	if change.enabled && (m.websocket == nil || change.addr != m.effectiveWebSocketAddr()) {
		replacement, err = m.listenWebSocket(change.addr, change.token)
		if err != nil {
			m.warnStatus(clean(err.Error()))
			return nil
		}
	}
	if m.websocketAddr == "" {
		cfg := m.config
		cfg.WebSocketEnabled = change.enabled
		cfg.WebSocketAddr = change.addr
		if err := m.store.SaveConfig(cfg); err != nil {
			if replacement != nil {
				err = errors.Join(err, replacement.Close())
			}
			m.warnStatus(clean(err.Error()))
			return nil
		}
		m.config = m.store.Config()
	} else {
		m.websocketSessionEnabled = change.enabled
	}
	// 新服务和配置都准备成功后才切换，失败时保留旧服务与地址。
	if replacement != nil || !change.enabled {
		old := m.websocket
		m.websocket = replacement
		m.setChatBroadcast()
		if old != nil {
			if err := old.Close(); err != nil {
				m.warn(err.Error())
			}
		}
	}
	if change.editingAddr {
		m.mode = ""
		m.input.SetValue("")
		m.input.Blur()
	}
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
	m.cacheWebSocketTokenStatus(token != "", nil)
	if m.websocket != nil {
		m.websocket.SetToken(token)
	}
	m.log(i18n.T(i18n.TUILogSettingsSaved))
	return m.finishResult()
}
