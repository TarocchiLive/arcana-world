// Package rtmpsrt 将 RTMP 字节流桥接到 SRT 消息传输，不使用 MPEG-TS。
package rtmpsrt

import (
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// Target 保留平台发布凭据的原始字节，不进行 URL 解码或重签。
type Target struct{ Host, App, Playpath, TCURL string }

var publishURL = regexp.MustCompile(`^rtmpsrt://([^/:?#\s]+)(:([0-9]{1,5}))?((/[^/?#]+){0,3}/)([^#\s]*)$`)

// Parse 按参考客户端的路径规则拆分完整 addr+code；不复制其截断凭据的缺陷。
func Parse(raw string) (Target, error) {
	m := publishURL.FindStringSubmatch(raw)
	if m == nil || strings.ContainsAny(raw, "\x00\r\n\t") {
		return Target{}, errors.New("invalid RTMPSRT publishing URL")
	}
	port := m[3]
	if port == "" {
		port = "1935"
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return Target{}, errors.New("invalid RTMPSRT port")
	}
	app := strings.Trim(m[4], "/")
	if app == "" || m[6] == "" || strings.Contains(m[1], "@") {
		return Target{}, errors.New("invalid RTMPSRT app or playpath")
	}
	host := m[1]
	if port != "1935" {
		host += ":" + port
	}
	return Target{Host: net.JoinHostPort(m[1], port), App: app, Playpath: m[6], TCURL: "rtmp://" + host + "/" + app}, nil
}
