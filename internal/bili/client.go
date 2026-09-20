// SPDX-License-Identifier: GPL-3.0-only
package bili

import (
	"arcana-world/internal/i18n"

	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"arcana-world/internal/domain"
)

const (
	appKey      = "aae92bc66f3edfab"
	appSecret   = "af125a0d5279fd576c1b4418a3e8276d"
	build       = "10783"
	version     = "7.63.0.10783"
	maxResponse = 4 << 20
)

const requestTimeout = 30 * time.Second
const liveWebOrigin = "https://live.bilibili.com"

// Client 同步执行操作。调用方负责串行处理账号变更和各项操作。
// 可替换基础 URL 和 HTTP 客户端以进行离线验证。
type Client struct {
	HTTP         *http.Client
	APIBase      string
	LiveBase     string
	PassportBase string
	account      domain.Account
	roomID       int64
	buvid        string
}

// APIError 绝不包含原始响应文本或账号机密。
type APIError struct {
	Code      int
	Operation string
}

func (e *APIError) Error() string {
	return fmt.Sprintf(i18n.T(i18n.BiliApiOperationFailed), e.Operation, e.Code)
}

// ValidationError 区分无效凭据与暂时故障。
// 这两类错误都不会删除或修改已保存的账号。
type ValidationError struct {
	Permanent bool
	Cause     error
}

func (e *ValidationError) Error() string {
	if e.Permanent {
		return i18n.T(i18n.BiliCredentialsExpired)
	}
	return i18n.T(i18n.BiliLoginValidationUnavailable)
}
func (e *ValidationError) Unwrap() error { return e.Cause }

func New(proxy string) (*Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	switch proxy {
	case "":
	case "direct":
		transport.Proxy = nil
	default:
		u, err := url.Parse(proxy)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
			return nil, errors.New(i18n.T(i18n.BiliProxyInvalid))
		}
		transport.Proxy = http.ProxyURL(u)
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return nil, errors.New(i18n.T(i18n.BiliDeviceIdentifierFailed))
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	buvid := fmt.Sprintf("%X-%X-%X-%X-%X%duser", id[:4], id[4:6], id[6:8], id[8:10], id[10:], os.Getpid())
	return &Client{HTTP: &http.Client{Transport: transport, Timeout: requestTimeout}, APIBase: "https://api.bilibili.com", LiveBase: "https://api.live.bilibili.com", PassportBase: "https://passport.bilibili.com", account: domain.Account{Cookies: make(map[string]string)}, buvid: buvid}, nil
}

func (c *Client) SetAccount(account domain.Account) {
	c.account = domain.Account{UID: account.UID, Name: account.Name, Cookies: make(map[string]string, len(account.Cookies))}
	for k, v := range account.Cookies {
		c.account.Cookies[k] = v
	}
	c.roomID = 0
}

// Python urllib.parse.urlencode 不转义 '~'，并用 '+' 表示空格。
func encode(v url.Values) string { return strings.ReplaceAll(v.Encode(), "%7E", "~") }
func signed(payload url.Values, omit ...string) url.Values {
	p := url.Values{"access_key": {""}, "build": {build}, "platform": {"pc_link"}, "ts": {decimal(time.Now().Unix())}, "version": {version}, "appkey": {appKey}}
	for _, key := range omit {
		p.Del(key)
	}
	for k, vs := range payload {
		p[k] = append([]string(nil), vs...)
	}
	sum := md5.Sum([]byte(encode(p) + appSecret))
	p.Set("sign", fmt.Sprintf("%x", sum))
	return p
}
func values(kv ...string) url.Values {
	v := make(url.Values, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		v.Set(kv[i], kv[i+1])
	}
	return v
}
func decimal(n int64) string { return strconv.FormatInt(n, 10) }
func (c *Client) csrf(p url.Values) (url.Values, error) {
	token := c.account.Cookies["bili_jct"]
	if token == "" || c.account.Cookies["SESSDATA"] == "" {
		return nil, errors.New(i18n.T(i18n.BiliSignInRequired))
	}
	p.Set("csrf", token)
	p.Set("csrf_token", token)
	return p, nil
}

type envelope struct {
	Code    *int            `json:"code"`
	Data    json.RawMessage `json:"data"`
	Message string          `json:"message"`
}

func (e envelope) check(op string) error {
	if e.Code == nil {
		return fmt.Errorf(i18n.T(i18n.BiliResponseStatusMissing), op)
	}
	if *e.Code != 0 {
		return &APIError{Code: *e.Code, Operation: op}
	}
	return nil
}
func decode(data json.RawMessage, out any) error {
	if len(data) == 0 || string(data) == "null" {
		return errors.New(i18n.T(i18n.BiliResponseDataMissing))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return errors.New(i18n.T(i18n.BiliResponseDataInvalid))
	}
	return nil
}

// 每个请求都使用不带 Cookie 容器的副本，既隔离匿名二维码请求，
// 也防止注入的 HTTP 客户端通过 Cookie 容器恢复过期账号。
// 拒绝所有重定向，包括可信主机之间的重定向：签名 POST 请求体
// 和 Cookie 请求头绝不能转发到其他目标。
func (c *Client) request(ctx context.Context, method, base, path string, query url.Values, body io.Reader, contentType string, anonymous, web bool) (envelope, []*http.Cookie, error) {
	var result envelope
	u, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil || u.Host == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return result, nil, errors.New(i18n.T(i18n.BiliApiUrlInvalid))
	}
	u.RawQuery = encode(query)
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return result, nil, errors.New(i18n.T(i18n.BiliApiRequestFailed))
	}
	req.Header.Set("Accept", "application/json")
	if c.buvid != "" {
		req.Header.Set("buvid", c.buvid)
	}
	req.Header.Set("User-Agent", "LiveHime/"+version+" os/Windows pc_app/livehime build/"+build+" osVer/10.0_x86_64")
	if web {
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/105.0.0.0 Safari/537.36 pc_app/livehime build/"+build)
		req.Header.Set("Origin", liveWebOrigin)
		req.Header.Set("Referer", liveWebOrigin+"/")
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	if !anonymous {
		req.AddCookie(&http.Cookie{Name: "appkey", Value: appKey})
		req.AddCookie(&http.Cookie{Name: "device_platform", Value: "Windows Version: 10.0 x86_64"})
		if c.account.Cookies["buvid3"] == "" && c.buvid != "" {
			req.AddCookie(&http.Cookie{Name: "buvid3", Value: c.buvid})
		}
		for k, v := range c.account.Cookies {
			cookie := &http.Cookie{Name: k, Value: v}
			if cookie.Valid() != nil {
				return result, nil, errors.New(i18n.T(i18n.BiliCookieInvalid))
			}
			req.AddCookie(cookie)
		}
	}
	if c.HTTP == nil {
		return result, nil, errors.New(i18n.T(i18n.BiliHttpClientMissing))
	}
	client := *c.HTTP
	client.Jar = nil
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return result, nil, ctx.Err()
		}
		return result, nil, errors.New(i18n.T(i18n.BiliNetworkFailed))
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return result, nil, fmt.Errorf(i18n.T(i18n.BiliApiHttpStatus), response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if ctx.Err() != nil {
		return result, nil, ctx.Err()
	}
	if err != nil {
		return result, nil, errors.New(i18n.T(i18n.BiliApiResponseReadFailed))
	}
	if len(raw) > maxResponse {
		return result, nil, errors.New(i18n.T(i18n.BiliApiResponseTooLarge))
	}
	if json.Unmarshal(raw, &result) != nil || result.Code == nil {
		return result, nil, errors.New(i18n.T(i18n.BiliApiResponseInvalid))
	}
	return result, response.Cookies(), nil
}
func (c *Client) call(ctx context.Context, method, base, path string, query, form url.Values, anonymous, web bool, out any) error {
	var body io.Reader
	contentType := ""
	if form != nil {
		body = strings.NewReader(encode(form))
		contentType = "application/x-www-form-urlencoded"
	}
	e, _, err := c.request(ctx, method, base, path, query, body, contentType, anonymous, web)
	if err != nil {
		return err
	}
	if err = e.check(path); err != nil {
		return err
	}
	if out != nil {
		return decode(e.Data, out)
	}
	return nil
}

// integer 接受 API 中可能使用十进制字符串或数字表示的字段。
type integer int64

func (n *integer) UnmarshalJSON(b []byte) error {
	s := string(b)
	if len(s) > 0 && s[0] == '"' {
		if json.Unmarshal(b, &s) != nil {
			return errors.New(i18n.T(i18n.BiliIntegerInvalid))
		}
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return errors.New(i18n.T(i18n.BiliIntegerInvalid))
	}
	*n = integer(v)
	return nil
}
