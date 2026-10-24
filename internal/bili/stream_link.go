// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"arcana-world/internal/journal"
	"arcana-world/internal/rtmpsrt"
)

type streamProtocol struct {
	Protocol string `json:"protocol"`
	Addr     string `json:"addr"`
	Code     string `json:"code"`
	NewLink  string `json:"new_link"`
}

func usableRTMPSRTEndpoint(address, key string) bool {
	_, err := rtmpsrt.Parse(address + key)
	return err == nil
}

// 动态节点地址保留原始 query；请求失败时继续使用同一记录的原地址。
func (c *Client) resolveStreamLink(ctx context.Context, p streamProtocol) (string, string) {
	if p.NewLink == "" || c.HTTP == nil {
		return p.Addr, p.Code
	}
	u, err := url.Parse(p.NewLink)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return p.Addr, p.Code
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.NewLink, nil)
	if err != nil {
		return p.Addr, p.Code
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "LiveHime/"+version+" os/Windows pc_app/livehime build/"+build+" osVer/10.0_x86_64")
	// 动态 URL 的登录态要求尚未确认，不向任意节点传递账号 Cookie 或跟随重定向。
	client := *c.HTTP
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		return p.Addr, p.Code
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return p.Addr, p.Code
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if err != nil || len(raw) > maxResponse {
		return p.Addr, p.Code
	}
	var result struct {
		Code *int `json:"code"`
		Data struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if json.Unmarshal(raw, &result) != nil || (result.Code != nil && *result.Code != 0) {
		return p.Addr, p.Code
	}
	journal.AddSecrets(result.Data.URL)
	address, query, found := strings.Cut(result.Data.URL, "?")
	key := ""
	if found {
		key = "?" + query
	}
	if !usableRTMPSRTEndpoint(address, key) {
		return p.Addr, p.Code
	}
	journal.AddSecrets(key)
	return address, key
}
