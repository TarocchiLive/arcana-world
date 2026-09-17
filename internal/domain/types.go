package domain

import (
	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	"arcana-world/internal/tts"
)

// Account 包含认证 Cookie，绝不能写入日志。
type Account struct {
	UID     string            `json:"uid"`
	Name    string            `json:"name"`
	Cookies map[string]string `json:"cookies"`
}
type AccountInfo struct {
	UID  string `json:"uid"`
	Name string `json:"name"`
}
type Config struct {
	ActiveUID      string        `json:"active_uid"`
	Accounts       []AccountInfo `json:"accounts"`
	Proxy          string        `json:"proxy"`    // 留空时使用环境变量；"direct" 禁用代理
	Protocol       string        `json:"protocol"` // 支持 rtmp、srt 和 srt-fallback
	OBSURL         string        `json:"obs_url"`
	OBSAutoConnect bool          `json:"obs_auto_connect"`
	OBSAutoStream  bool          `json:"obs_auto_stream"`
	// 退出清理默认启用；旧配置缺省值也会停止推流并关闭直播间。
	ExitOBSStopDisabled  bool `json:"exit_obs_stop_disabled"`
	ExitLiveStopDisabled bool `json:"exit_live_stop_disabled"`
	// 默认启用；使用禁用字段使旧配置无需迁移即可自动监听。
	DanmakuDisabled       bool             `json:"danmaku_disabled"`
	RecentTitles          []string         `json:"recent_titles"`
	RecentAreas           []Area           `json:"recent_areas"`
	Overlay               overlay.Settings `json:"overlay"`
	OverlayDisabledEvents []string         `json:"overlay_disabled_events,omitempty"`
	TTS                   tts.Settings     `json:"tts"`
}
type Room struct {
	ID           int64
	Title        string
	AreaID       int64
	AreaName     string
	ParentName   string
	Live         bool
	CoverURL     string
	CoverStatus  string
	Announcement string
}
type Area struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Parent string `json:"parent"`
}
type QR struct {
	Key string
	URL string
}
type LoginPoll struct {
	Code    int
	Account *Account
}
type Stream struct {
	Address  string
	Key      string
	Protocol string
	Warning  string
}

// FaceChallenge 请求用户完成 B 站的真实身份验证。
type FaceChallenge struct {
	URL     string
	Voucher string
	Message string
}

func (f *FaceChallenge) Error() string {
	return i18n.T(i18n.DomainIdentityVerificationRequired)
}

type OBSStatus struct {
	Active       bool
	Reconnecting bool
	Timecode     string
}
