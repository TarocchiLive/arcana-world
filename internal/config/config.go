// Package config 定义应用配置；基础领域类型不依赖此包。
package config

import (
	"arcana-world/internal/domain"
	"arcana-world/internal/overlay"
	"arcana-world/internal/presentation"
	"arcana-world/internal/tts"
)

// Config 保存应用偏好和账号索引，不包含登录凭据。
type Config struct {
	ActiveUID string               `json:"active_uid"`
	Accounts  []domain.AccountInfo `json:"accounts"`
	Proxy     string               `json:"proxy"`    // 留空时使用环境变量；"direct" 禁用代理
	Protocol  string               `json:"protocol"` // 支持 rtmp、srt、srt-fallback 和 rtmpsrt
	TUITheme  string               `json:"tui_theme,omitempty"`
	// 空值与 tui 均使用 TUI；webui 使用浏览器界面。
	DefaultUI string `json:"default_ui,omitempty"`
	// 默认仅监听本机；手动改为 0.0.0.0 后，局域网设备可直接控制应用。
	WebBindAddress string `json:"web_bind_address"`
	// 零值保留默认动效、完整标题和结果通知。
	TUIMotionDisabled            bool   `json:"tui_motion_disabled,omitempty"`
	TUICompactHeader             bool   `json:"tui_compact_header,omitempty"`
	TUINotificationsWarningsOnly bool   `json:"tui_notifications_warnings_only,omitempty"`
	OBSURL                       string `json:"obs_url"`
	OBSAutoConnect               bool   `json:"obs_auto_connect"`
	OBSAutoStream                bool   `json:"obs_auto_stream"`
	// 退出清理默认启用；旧配置缺省值也会停止推流并关闭直播间。
	ExitOBSStopDisabled  bool `json:"exit_obs_stop_disabled"`
	ExitLiveStopDisabled bool `json:"exit_live_stop_disabled"`
	// 默认启用；使用禁用字段使旧配置无需迁移即可自动监听。
	DanmakuDisabled       bool             `json:"danmaku_disabled"`
	DanmakuLimit          int              `json:"danmaku_limit"`
	DanmakuShowOther      bool             `json:"danmaku_show_other"`
	WebSocketEnabled      bool             `json:"websocket_enabled"`
	WebSocketAddr         string           `json:"websocket_addr"`
	RecentTitles          []string         `json:"recent_titles"`
	RecentAreas           []domain.Area    `json:"recent_areas"`
	Overlay               overlay.Settings `json:"overlay"`
	OverlayDisabledEvents []string         `json:"overlay_disabled_events,omitempty"`
	TTS                   tts.Settings     `json:"tts"`
	// 屏蔽词属于用户整理的内容，重置设置时保留。
	Blocklist presentation.Blocklist `json:"blocklist"`
}
