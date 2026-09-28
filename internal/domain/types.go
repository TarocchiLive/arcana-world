package domain

import "arcana-world/internal/i18n"

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
