// SPDX-License-Identifier: GPL-3.0-only
// Protocol reference: xfgryujk/blivedm (MIT), clients/web.py and clients/ws_base.py:
// https://github.com/xfgryujk/blivedm/tree/dev/blivedm/clients
// This is an independent implementation; no anonymous or stale-host fallback.
package bili

import (
	"bufio"
	"context"
	"crypto/md5"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

const danmakuUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

// The wrapper reuses request's cookie validation, bounded reads and redirect
// rejection while avoiding its desktop device identifier and desktop UA.
type danmakuTransport struct{ base http.RoundTripper }

func (t danmakuTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("User-Agent", danmakuUserAgent)
	return t.base.RoundTrip(r)
}

func danmakuWBIKey(img, sub string) (string, error) {
	keyPart := func(s string) string {
		u, e := url.Parse(s)
		if e != nil {
			return ""
		}
		return strings.SplitN(path.Base(u.Path), ".", 2)[0]
	}
	key := keyPart(img) + keyPart(sub)
	if len(key) != 64 {
		return "", errors.New("danmaku: invalid WBI keys")
	}
	indexes := [...]int{46, 47, 18, 2, 53, 8, 23, 32, 15, 50, 10, 31, 58, 3, 45, 35, 27, 43, 5, 49, 33, 9, 42, 19, 29, 28, 14, 39, 12, 38, 41, 13}
	var mixed [32]byte
	for i, index := range indexes {
		mixed[i] = key[index]
	}
	return string(mixed[:]), nil
}
func danmakuWBISign(query url.Values, key string, now time.Time) url.Values {
	p := make(url.Values, len(query)+2)
	for k, v := range query {
		p[k] = append([]string(nil), v...)
	}
	p.Del("w_rid")
	p.Set("wts", decimal(now.Unix()))
	for k, vs := range p {
		for i, v := range vs {
			p[k][i] = strings.Map(func(r rune) rune {
				if strings.ContainsRune("!'()*", r) {
					return -1
				}
				return r
			}, v)
		}
	}
	sum := md5.Sum([]byte(encode(p) + key))
	p.Set("w_rid", fmt.Sprintf("%x", sum))
	return p
}
func danmakuHost(host string) bool {
	if !strings.HasSuffix(host, ".chat.bilibili.com") {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
				return false
			}
		}
	}
	return true
}

// ListenDanmaku performs one authenticated connection attempt. The caller owns
// retries and must keep this account snapshot unchanged until it returns.
// receive runs synchronously and may retain its payload. Its error is returned
// unchanged, including when cancellation occurs at the same time.
func (c *Client) ListenDanmaku(ctx context.Context, roomID int64, attempt int, authenticated func(), receive func(json.RawMessage) error) error {
	if roomID <= 0 || receive == nil {
		return errors.New("danmaku: invalid listener arguments")
	}
	if c.account.Cookies["SESSDATA"] == "" {
		return errors.New("danmaku: sign in required")
	}
	if c.HTTP == nil {
		return errors.New("danmaku: HTTP client missing")
	}
	transport := c.HTTP.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	tr, ok := transport.(*http.Transport)
	if !ok {
		return errors.New("danmaku: unsupported HTTP transport")
	}
	client := *c
	hc := *c.HTTP
	hc.Transport = danmakuTransport{transport}
	client.HTTP = &hc
	client.buvid = ""
	client.SetAccount(c.account)
	var nav struct {
		IsLogin bool    `json:"isLogin"`
		MID     integer `json:"mid"`
		WBI     struct {
			Img string `json:"img_url"`
			Sub string `json:"sub_url"`
		} `json:"wbi_img"`
	}
	if err := client.call(ctx, http.MethodGet, c.APIBase, "/x/web-interface/nav", nil, nil, false, true, &nav); err != nil {
		return err
	}
	if !nav.IsLogin || nav.MID <= 0 || (c.account.UID != "" && decimal(int64(nav.MID)) != c.account.UID) {
		return errors.New("danmaku: account validation failed")
	}
	key, err := danmakuWBIKey(nav.WBI.Img, nav.WBI.Sub)
	if err != nil {
		return err
	}
	buvid := client.account.Cookies["buvid3"]
	if buvid == "" {
		var fingerprint struct {
			B3 string `json:"b_3"`
		}
		if err := client.call(ctx, http.MethodGet, c.APIBase, "/x/frontend/finger/spi", nil, nil, true, true, &fingerprint); err != nil {
			return err
		}
		buvid = fingerprint.B3
		if buvid == "" || (&http.Cookie{Name: "buvid3", Value: buvid}).Valid() != nil {
			return errors.New("danmaku: invalid web fingerprint")
		}
		client.account.Cookies["buvid3"] = buvid
	}
	var room struct {
		RoomID integer `json:"room_id"`
	}
	if err := client.call(ctx, http.MethodGet, c.LiveBase, "/room/v1/Room/get_info", values("room_id", decimal(roomID)), nil, false, true, &room); err != nil {
		return err
	}
	if room.RoomID <= 0 {
		return errors.New("danmaku: invalid room")
	}
	var info struct {
		Token string `json:"token"`
		Hosts []struct {
			Host string `json:"host"`
			Port int    `json:"wss_port"`
		} `json:"host_list"`
	}
	query := danmakuWBISign(values("id", decimal(int64(room.RoomID)), "type", "0"), key, time.Now())
	if err := client.call(ctx, http.MethodGet, c.LiveBase, "/xlive/web-room/v1/index/getDanmuInfo", query, nil, false, true, &info); err != nil {
		return err
	}
	hosts := make([]string, 0, len(info.Hosts))
	for _, h := range info.Hosts {
		if danmakuHost(h.Host) && h.Port > 0 && h.Port <= 65535 {
			hosts = append(hosts, net.JoinHostPort(h.Host, decimal(int64(h.Port))))
		}
	}
	if len(hosts) == 0 || info.Token == "" {
		return errors.New("danmaku: invalid connection metadata")
	}
	index := attempt % len(hosts)
	if index < 0 {
		index += len(hosts)
	}
	target := "wss://" + hosts[index] + "/sub"
	dialer, err := danmakuDialer(ctx, tr, target)
	if err != nil {
		return err
	}
	headers := http.Header{"User-Agent": {danmakuUserAgent}, "Origin": {"https://live.bilibili.com"}, "Referer": {"https://live.bilibili.com/"}}
	// Cookies are only used at the metadata endpoints, never sent to comet hosts.
	conn, response, err := dialer.DialContext(ctx, target, headers)
	if response != nil && response.Body != nil {
		response.Body.Close()
	}
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("danmaku: websocket connection failed")
	}
	auth, _ := json.Marshal(map[string]any{"uid": int64(nav.MID), "roomid": int64(room.RoomID), "protover": 3, "platform": "web", "type": 2, "key": info.Token, "buvid": buvid})
	return danmakuSession(ctx, conn, auth, authenticated, receive, 30*time.Second, 10*time.Second)
}

func danmakuDialer(ctx context.Context, tr *http.Transport, target string) (*websocket.Dialer, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if tr.TLSClientConfig != nil {
		config = tr.TLSClientConfig.Clone()
		config.InsecureSkipVerify = false
		config.ServerName = ""
		if config.MinVersion < tls.VersionTLS12 {
			config.MinVersion = tls.VersionTLS12
		}
	}
	// Both Gorilla Upgrade and our proxy CONNECT use HTTP/1.1, not HTTP/2.
	config.NextProtos = []string{"http/1.1"}
	dial := tr.DialContext
	if dial == nil {
		dial = (&net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	}
	underlyingDial := dial
	dial = func(dialCtx context.Context, network, addr string) (net.Conn, error) {
		raw, err := underlyingDial(dialCtx, network, addr)
		if err != nil {
			return nil, err
		}
		closed := make(chan struct{})
		wrapped := &danmakuContextConn{Conn: raw, canceled: closed}
		wrapped.stop = context.AfterFunc(ctx, func() { raw.Close(); close(closed) })
		return wrapped, nil
	}
	d := &websocket.Dialer{NetDialContext: dial, TLSClientConfig: config, HandshakeTimeout: 30 * time.Second}
	if tr.Proxy == nil {
		return d, nil
	}
	u, _ := url.Parse(target)
	u.Scheme = "https"
	proxy, err := tr.Proxy(&http.Request{URL: u})
	if err != nil {
		return nil, errors.New("danmaku: proxy configuration failed")
	}
	if proxy == nil {
		return d, nil
	}
	p := *proxy
	switch p.Scheme {
	case "socks5", "socks5h":
		p.Scheme = "socks5"
		if p.Port() == "" {
			p.Host = net.JoinHostPort(p.Hostname(), "1080")
		}
		d.Proxy = http.ProxyURL(&p)
	case "http", "https":
		port := p.Port()
		if port == "" {
			port = "80"
			if p.Scheme == "https" {
				port = "443"
			}
		}
		proxyAddress := net.JoinHostPort(p.Hostname(), port)
		proxyTLS := config.Clone()
		proxyTLS.ServerName = p.Hostname()
		d.NetDialContext = func(dialCtx context.Context, network, addr string) (net.Conn, error) {
			raw, err := dial(dialCtx, network, proxyAddress)
			if err != nil {
				return nil, err
			}
			if deadline, ok := dialCtx.Deadline(); ok {
				if err := raw.SetDeadline(deadline); err != nil {
					raw.Close()
					return nil, err
				}
			}
			if p.Scheme == "https" {
				secured := tls.Client(raw, proxyTLS)
				if err := secured.HandshakeContext(dialCtx); err != nil {
					raw.Close()
					return nil, err
				}
				raw = secured
			}
			if err := danmakuCONNECT(raw, addr, p.User); err != nil {
				raw.Close()
				return nil, err
			}
			return raw, nil
		}
	default:
		return nil, errors.New("danmaku: unsupported proxy")
	}
	return d, nil
}

// Do not delegate CONNECT to Gorilla v1.5.3: its rejection path indexes an
// optional reason phrase and can panic on a valid reason-less status line.
func danmakuCONNECT(conn net.Conn, address string, user *url.Userinfo) error {
	headers := make(http.Header)
	if user != nil {
		password, _ := user.Password()
		credentials := base64.StdEncoding.EncodeToString([]byte(user.Username() + ":" + password))
		headers.Set("Proxy-Authorization", "Basic "+credentials)
	}
	request := &http.Request{
		Method: http.MethodConnect, URL: &url.URL{Opaque: address}, Host: address, Header: headers,
	}
	if err := request.Write(conn); err != nil {
		return errors.New("danmaku: proxy CONNECT write failed")
	}
	limited := &io.LimitedReader{R: conn, N: 64 << 10}
	reader := bufio.NewReader(limited)
	response, err := http.ReadResponse(reader, request)
	if err != nil || limited.N == 0 {
		return errors.New("danmaku: invalid proxy CONNECT response")
	}
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("danmaku: proxy CONNECT rejected (%d)", response.StatusCode)
	}
	// The TLS peer cannot send application bytes before our ClientHello.
	if reader.Buffered() != 0 {
		return errors.New("danmaku: unexpected data after proxy CONNECT")
	}
	return nil
}

// Cancellation also interrupts CONNECT/SOCKS and TLS negotiation, before the
// websocket reader exists. Close joins the cancellation callback if it ran.
type danmakuContextConn struct {
	net.Conn
	stop     func() bool
	canceled <-chan struct{}
	once     sync.Once
	err      error
}

func (c *danmakuContextConn) Close() error {
	c.once.Do(func() {
		if !c.stop() {
			<-c.canceled
		}
		c.err = c.Conn.Close()
	})
	return c.err
}

func danmakuSession(ctx context.Context, conn *websocket.Conn, auth []byte, authenticated func(), receive func(json.RawMessage) error, interval, timeout time.Duration) error {
	defer conn.Close()
	conn.SetReadLimit(danmakuMaxPacket)
	done := make(chan struct{})
	joined := make(chan struct{})
	startHeartbeat := make(chan struct{})
	var heartbeatFailed atomic.Bool
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
			conn.Close()
			return
		case <-done:
			return
		case <-startHeartbeat:
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				conn.Close()
				return
			case <-done:
				return
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(timeout))
				if conn.WriteMessage(websocket.BinaryMessage, danmakuPacket(2, 1, nil)) != nil {
					heartbeatFailed.Store(true)
					conn.Close()
					return
				}
			}
		}
	}()
	defer func() { close(done); <-joined }()
	conn.SetWriteDeadline(time.Now().Add(timeout))
	if err := conn.WriteMessage(websocket.BinaryMessage, danmakuPacket(7, 1, auth)); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("danmaku: authentication write failed")
	}
	conn.SetReadDeadline(time.Now().Add(timeout))
	authed := false
	for {
		typ, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if heartbeatFailed.Load() {
				return errors.New("danmaku: heartbeat write failed")
			}
			return errors.New("danmaku: connection closed or heartbeat acknowledgement timed out")
		}
		if typ != websocket.BinaryMessage {
			return errors.New("danmaku: unexpected websocket message")
		}
		budget := danmakuMaxExpanded
		err = danmakuDecode(data, 0, &budget, func(op uint32, body []byte) error {
			switch op {
			case 8:
				var reply struct {
					Code *int `json:"code"`
				}
				if json.Unmarshal(body, &reply) != nil || reply.Code == nil || *reply.Code != 0 {
					return errors.New("danmaku: server authentication rejected")
				}
				if !authed {
					authed = true
					conn.SetReadDeadline(time.Now().Add(interval + timeout))
					close(startHeartbeat)
					if authenticated != nil {
						authenticated()
					}
				}
			case 3:
				if authed {
					conn.SetReadDeadline(time.Now().Add(interval + timeout))
				}
			case 5:
				// Do not consult ctx here: completed packets must reach durable storage,
				// including earlier packets in a frame with a later corrupt packet.
				return receive(append(json.RawMessage(nil), body...))
			}
			return nil
		})
		if err != nil {
			return err
		}
	}
}
