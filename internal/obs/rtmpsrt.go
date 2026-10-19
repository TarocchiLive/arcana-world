package obs

import (
	"context"
	"errors"
	"net"
	"net/url"
	"strings"

	"arcana-world/internal/i18n"
	"arcana-world/internal/rtmpsrt"
)

// CheckRTMPSRT 在平台开播前检查本机 OBS 和可选运行库，避免创建无法使用的直播。
func (c *Client) CheckRTMPSRT() error {
	s, err := c.current()
	if err != nil {
		return err
	}
	u, err := url.Parse(s.endpoint)
	if err != nil {
		return errors.New(i18n.T(i18n.OBSRTMPSRTLocalOnly))
	}
	host := u.Hostname()
	ip := net.ParseIP(host)
	if !strings.EqualFold(host, "localhost") && (ip == nil || !ip.IsLoopback()) {
		return errors.New(i18n.T(i18n.OBSRTMPSRTLocalOnly))
	}
	if err := rtmpsrt.Available(); err != nil {
		return errors.New(i18n.T(i18n.OBSRTMPSRTUnavailable))
	}
	return nil
}
func (c *Client) closeBridgeLocked() {
	if c.bridge != nil {
		c.bridge.Close()
		pending := c.closingBridges[:0]
		for _, done := range c.closingBridges {
			select {
			case <-done:
			default:
				pending = append(pending, done)
			}
		}
		c.closingBridges = append(pending, c.bridge.Done())
		c.bridge = nil
	}
}

func (c *Client) waitBridges() {
	c.mu.Lock()
	pending := append([]<-chan struct{}(nil), c.closingBridges...)
	c.mu.Unlock()
	for _, done := range pending {
		<-done
	}
}
func (c *Client) installBridge(ctx context.Context, s *session, target rtmpsrt.Target) error {
	// 错误回调只报告当前桥；旧桥退出不能覆盖新会话状态。
	var bridge *rtmpsrt.Bridge
	ready := make(chan struct{})
	var err error
	bridge, err = rtmpsrt.New(target, func(error) {
		<-ready
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.session == s && c.bridge == bridge && !c.closed {
			c.snapshot.Err = errors.New(i18n.T(i18n.OBSRTMPSRTFailed))
			c.publishLocked()
		}
	})
	close(ready)
	if err != nil {
		return errors.New(i18n.T(i18n.OBSRTMPSRTFailed))
	}
	c.mu.Lock()
	if c.session != s || c.closed {
		c.mu.Unlock()
		bridge.Close()
		<-bridge.Done()
		return errors.New(i18n.T(i18n.OBSDisconnected))
	}
	c.closeBridgeLocked()
	c.bridge = bridge
	c.mu.Unlock()
	c.waitBridges()
	_, err = c.request(ctx, s, "SetStreamServiceSettings", map[string]any{
		"streamServiceType":     "rtmp_custom",
		"streamServiceSettings": map[string]any{"bwtest": false, "server": bridge.Server(), "key": bridge.Key(), "use_auth": false},
	})
	if err != nil {
		c.mu.Lock()
		if c.bridge == bridge {
			c.closeBridgeLocked()
		}
		c.mu.Unlock()
		c.waitBridges()
	}
	return err
}
