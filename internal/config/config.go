// Package config 定义应用配置；基础领域类型不依赖此包。
package config

import (
	"arcana-world/internal/domain"
	"arcana-world/internal/overlay"
	"arcana-world/internal/tts"
)

// Config 保存应用偏好和账号索引，不包含登录凭据。
type Config struct {
	ActiveUID string               `json:"active_uid"`
	Accounts  []domain.AccountInfo `json:"accounts"`
	Proxy     string               `json:"proxy"`    // 留空时使用环境变量；"direct" 禁用代理
	Protocol  string               `json:"protocol"` // 支持 rtmp、srt 和 srt-fallback
	TUITheme  string               `json:"tui_theme,omitempty"`
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
	DanmakuDisabled  bool `json:"danmaku_disabled"`
	DanmakuLimit     int  `json:"danmaku_limit"`
	DanmakuShowOther bool `json:"danmaku_show_other"`
	// 默认忽略自身；缺省字段兼容旧配置。
	AudienceIncludeSelf   bool             `json:"audience_include_self,omitempty"`
	RecentTitles          []string         `json:"recent_titles"`
	RecentAreas           []domain.Area    `json:"recent_areas"`
	Overlay               overlay.Settings `json:"overlay"`
	OverlayDisabledEvents []string         `json:"overlay_disabled_events,omitempty"`
	TTS                   tts.Settings     `json:"tts"`
}
