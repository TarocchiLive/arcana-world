package domain

// Account contains authenticated cookies; never include it in logs.
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
	Proxy          string        `json:"proxy"`    // empty uses environment; "direct" disables proxies
	Protocol       string        `json:"protocol"` // rtmp, srt, srt-fallback
	OBSURL         string        `json:"obs_url"`
	OBSAutoConnect bool          `json:"obs_auto_connect"`
	OBSAutoStream  bool          `json:"obs_auto_stream"`
	RecentTitles   []string      `json:"recent_titles"`
	RecentAreas    []Area        `json:"recent_areas"`
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

// FaceChallenge asks the user to complete Bilibili's real identity verification.
type FaceChallenge struct {
	URL     string
	Voucher string
	Message string
}

func (f *FaceChallenge) Error() string {
	return "B 站要求身份验证，请使用二维码或链接完成验证后重新开播"
}

type OBSStatus struct {
	Active       bool
	Reconnecting bool
	Timecode     string
}
