// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
	"arcana-world/internal/i18n"

	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"arcana-world/internal/domain"
)

func (c *Client) GenerateQR(ctx context.Context) (domain.QR, error) {
	now := time.Now()
	p := values("source", "live_pc", "web_location", "0.0", "go_url", fmt.Sprintf("https://live.bilibili.com/p/html/live-pc-blink/mini-login-v2/?livehime_create_ts=%d&livehime_ts=%d", now.UnixMilli(), now.Unix()))
	var d struct {
		Key string `json:"qrcode_key"`
		URL string `json:"url"`
	}
	err := c.call(ctx, http.MethodGet, c.PassportBase, "/x/passport-login/web/qrcode/generate", p, nil, true, true, &d)
	if err != nil {
		return domain.QR{}, err
	}
	if d.Key == "" || !validLink(d.URL) {
		return domain.QR{}, errors.New(i18n.T(i18n.BiliLoginQrIncomplete))
	}
	return domain.QR{Key: d.Key, URL: d.URL}, nil
}
func (c *Client) PollQR(ctx context.Context, key string) (domain.LoginPoll, error) {
	if key == "" {
		return domain.LoginPoll{}, errors.New(i18n.T(i18n.BiliLoginQrKeyEmpty))
	}
	e, cookies, err := c.request(ctx, http.MethodGet, c.PassportBase, "/x/passport-login/web/qrcode/poll", values("qrcode_key", key, "source", "live_pc", "web_location", "0.0"), nil, "", true, true)
	if err != nil {
		return domain.LoginPoll{}, err
	}
	if err = e.check(i18n.T(i18n.BiliQrLogin)); err != nil {
		return domain.LoginPoll{}, err
	}
	var d struct {
		Code *int `json:"code"`
	}
	if err = decode(e.Data, &d); err != nil {
		return domain.LoginPoll{}, err
	}
	if d.Code == nil {
		return domain.LoginPoll{}, errors.New(i18n.T(i18n.BiliLoginQrStatusMissing))
	}
	result := domain.LoginPoll{Code: *d.Code}
	switch *d.Code {
	case 86101, 86038, 86090:
		return result, nil
	case 0:
		account := domain.Account{Cookies: make(map[string]string)}
		for _, cookie := range cookies {
			if cookie.MaxAge >= 0 {
				account.Cookies[cookie.Name] = cookie.Value
			}
		}
		account.UID = account.Cookies["DedeUserID"]
		if account.UID == "" || account.Cookies["SESSDATA"] == "" || account.Cookies["bili_jct"] == "" {
			return result, errors.New(i18n.T(i18n.BiliLoginCookiesMissing))
		}
		result.Account = &account
		return result, nil
	default:
		return result, &APIError{Code: *d.Code, Operation: i18n.T(i18n.BiliQrLogin)}
	}
}
func (c *Client) Validate(ctx context.Context) (domain.AccountInfo, error) {
	var d struct {
		IsLogin *bool   `json:"isLogin"`
		MID     integer `json:"mid"`
		Name    *string `json:"uname"`
	}
	err := c.call(ctx, http.MethodGet, c.APIBase, "/x/web-interface/nav", signed(nil, "access_key", "build", "version"), nil, false, true, &d)
	if err != nil {
		var api *APIError
		return domain.AccountInfo{}, &ValidationError{Permanent: errors.As(err, &api) && api.Code == -101, Cause: err}
	}
	if d.IsLogin == nil {
		return domain.AccountInfo{}, &ValidationError{}
	}
	if !*d.IsLogin {
		return domain.AccountInfo{}, &ValidationError{Permanent: true}
	}
	if d.MID <= 0 || d.Name == nil {
		return domain.AccountInfo{}, &ValidationError{}
	}
	uid := decimal(int64(d.MID))
	expected := c.account.UID
	if expected == "" {
		expected = c.account.Cookies["DedeUserID"]
	}
	if expected == "" || uid != expected {
		return domain.AccountInfo{}, &ValidationError{Permanent: true}
	}
	return domain.AccountInfo{UID: uid, Name: *d.Name}, nil
}
func validLink(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.User == nil && (u.Hostname() == "bilibili.com" || strings.HasSuffix(u.Hostname(), ".bilibili.com"))
}
func quote(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(url.QueryEscape(s), "+", "%20"), "%2F", "/")
}

func (c *Client) ensureDevice(ctx context.Context) error {
	if _, err := c.csrf(values()); err != nil {
		return err
	}
	expires, _ := strconv.ParseInt(c.account.Cookies["bili_ticket_expires"], 10, 64)
	if c.account.Cookies["bili_ticket"] == "" || expires <= time.Now().Unix() {
		ts := decimal(time.Now().Unix())
		mac := hmac.New(sha256.New, []byte("XgwSnGZ1p"))
		mac.Write([]byte("ts" + ts))
		p := values("key_id", "ec02", "hexsign", fmt.Sprintf("%x", mac.Sum(nil)), "context[ts]", ts, "csrf", c.account.Cookies["bili_jct"])
		var d struct {
			Ticket  string  `json:"ticket"`
			Created integer `json:"created_at"`
			TTL     integer `json:"ttl"`
		}
		if err := c.call(ctx, http.MethodPost, c.APIBase, "/bapis/bilibili.api.ticket.v1.Ticket/GenWebTicket", p, nil, false, true, &d); err != nil {
			return err
		}
		if d.Ticket == "" || d.Created <= 0 || d.TTL <= 0 || int64(d.Created) > 1<<62 || int64(d.TTL) > 1<<62 {
			return errors.New(i18n.T(i18n.BiliDeviceTicketInvalid))
		}
		c.account.Cookies["bili_ticket"] = d.Ticket
		c.account.Cookies["bili_ticket_expires"] = decimal(int64(d.Created + d.TTL))
	}
	if c.account.Cookies["buvid3"] == "" || c.account.Cookies["buvid4"] == "" {
		var d struct {
			B3 string `json:"b_3"`
			B4 string `json:"b_4"`
		}
		if err := c.call(ctx, http.MethodGet, c.APIBase, "/x/frontend/finger/spi", nil, nil, false, true, &d); err != nil {
			return err
		}
		if d.B3 == "" || d.B4 == "" {
			return errors.New(i18n.T(i18n.BiliDeviceResponseInvalid))
		}
		c.account.Cookies["buvid3"] = d.B3
		c.account.Cookies["buvid4"] = quote(d.B4)
	}
	return nil
}
