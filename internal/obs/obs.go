// obs 包实现持久化 OBS WebSocket v5 会话。
package obs

import (
	"arcana-world/internal/i18n"
	"arcana-world/internal/journal"

	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"sync"
	"time"

	"arcana-world/internal/domain"
	"github.com/gorilla/websocket"
)

const operationTimeout = 15 * time.Second

var logger = journal.Logger("obs")

// OBS WebSocket v5 操作码及协商后的协议设置。
const (
	opHello           = 0
	opIdentify        = 1
	opIdentified      = 2
	opEvent           = 5
	opRequest         = 6
	opRequestResponse = 7
	rpcVersion        = 1
	subscribeOutputs  = 1 << 6
	requestSucceeded  = 100
)

type Snapshot struct {
	Connected         bool
	Connecting        bool
	Status            domain.OBSStatus
	Err               error
	NeedsStreamStatus bool // 开播请求可能已生效，需按序查询后确认状态。
}

type Client struct {
	mu            sync.Mutex
	snapshot      Snapshot
	events        chan Snapshot
	session       *session
	generation    uint64
	cancel        context.CancelFunc
	closed        bool
	operations    chan struct{}
	startRevision uint64
}

type session struct {
	ws       *websocket.Conn
	done     chan struct{}
	endpoint string
	host     string // 仅 host:port，用于日志
	writes   chan struct{}
	pending  map[string]*pending
	next     uint64
}
type pending struct {
	kind          string
	result        chan reply
	startRevision uint64
}
type reply struct {
	data json.RawMessage
	err  error
}
type envelope struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
}

func NewClient() *Client {
	return &Client{events: make(chan Snapshot, 1), operations: make(chan struct{}, 1)}
}
func (c *Client) Snapshot() Snapshot      { c.mu.Lock(); defer c.mu.Unlock(); return c.snapshot }
func (c *Client) Events() <-chan Snapshot { return c.events }

// publishLocked 合并状态更新；Snapshot 始终是权威状态。
func (c *Client) publishLocked() {
	select {
	case c.events <- c.snapshot:
	default:
		select {
		case <-c.events:
		default:
		}
		select {
		case c.events <- c.snapshot:
		default:
		}
	}
}

// transportError 记录底层传输错误（含 WebSocket 关闭码/原因），返回不含技术细节的用户错误。
func transportError(ctx context.Context, log *slog.Logger, op, action string, cause error) error {
	attrs := []any{"op", op, "err", cause}
	var closeErr *websocket.CloseError
	if errors.As(cause, &closeErr) {
		attrs = append(attrs, "close_code", closeErr.Code, "close_reason", closeErr.Text)
	}
	if err := ctx.Err(); err != nil {
		log.Warn("obs operation interrupted", append(attrs, "ctx_err", err)...)
		return fmt.Errorf(i18n.T(i18n.OBSOperationContextError), action, err)
	}
	log.Error("obs transport failed", attrs...)
	// 服务器关闭原因、URL 和响应说明可能回显机密信息，不放入用户错误。
	return fmt.Errorf(i18n.T(i18n.OBSTransportFailed), action)
}

func (c *Client) Connect(ctx context.Context, endpoint, password string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "ws" && u.Scheme != "wss") || u.User != nil || u.Fragment != "" {
		// 端点可能含用户信息，仅记录结构特征。
		scheme, hasUser := "", false
		if u != nil {
			scheme, hasUser = u.Scheme, u.User != nil
		}
		logger.Warn("obs endpoint invalid", "parse_failed", err != nil, "scheme", scheme, "has_userinfo", hasUser)
		return errors.New(i18n.T(i18n.OBSEndpointInvalid))
	}
	journal.AddSecrets(password)
	log := logger.With("endpoint", u.Host)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		log.Warn("obs connect rejected", "reason", "client_closed")
		return errors.New(i18n.T(i18n.OBSClientClosed))
	}
	if c.snapshot.Connecting {
		c.mu.Unlock()
		log.Warn("obs connect rejected", "reason", "in_progress")
		return errors.New(i18n.T(i18n.OBSConnectionInProgress))
	}
	// 连接设置仅在明确断开连接后生效。
	if c.session != nil {
		same := c.session.endpoint == endpoint
		current := c.session.host
		c.mu.Unlock()
		if !same {
			log.Warn("obs connect rejected", "reason", "endpoint_change_requires_disconnect", "current", current)
			return errors.New(i18n.T(i18n.OBSEndpointChangeRequiresDisconnect))
		}
		return nil
	}
	handshake, cancel := context.WithTimeout(ctx, operationTimeout)
	c.generation++
	generation := c.generation
	c.cancel = cancel
	c.snapshot = Snapshot{Connecting: true, Status: c.snapshot.Status, NeedsStreamStatus: c.snapshot.NeedsStreamStatus}
	c.publishLocked()
	c.mu.Unlock()
	defer cancel()
	start := time.Now()
	log.Info("obs connecting", "password_set", password != "")
	ws, response, dialErr := (&websocket.Dialer{HandshakeTimeout: operationTimeout}).DialContext(handshake, endpoint, nil)
	if dialErr != nil {
		dialLog := log.With("elapsed", time.Since(start))
		if response != nil {
			dialLog = dialLog.With("http_status", response.StatusCode)
			if response.Body != nil {
				response.Body.Close()
			}
		}
		err = transportError(handshake, dialLog, "dial", i18n.T(i18n.OBSConnect), dialErr)
	} else {
		stopped := make(chan struct{})
		stop := context.AfterFunc(handshake, func() { ws.Close(); close(stopped) })
		ws.SetReadLimit(1 << 20)
		deadline, _ := handshake.Deadline()
		if derr := setDeadlines(ws, deadline); derr != nil {
			err = transportError(handshake, log, "set_handshake_deadline", i18n.T(i18n.OBSSetHandshakeDeadline), derr)
		} else {
			err = identify(handshake, ws, password, u.Host)
		}
		if !stop() {
			<-stopped
			err = transportError(handshake, log, "handshake_interrupted", i18n.T(i18n.OBSConnect), handshake.Err())
		}
		if err == nil && handshake.Err() != nil {
			err = transportError(handshake, log, "handshake_interrupted", i18n.T(i18n.OBSConnect), handshake.Err())
		}
		if err == nil {
			if derr := setDeadlines(ws, time.Time{}); derr != nil {
				err = transportError(handshake, log, "finish_handshake", i18n.T(i18n.OBSFinishHandshake), derr)
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation || c.closed {
		if ws != nil {
			ws.Close()
		}
		log.Info("obs connect superseded", "elapsed", time.Since(start))
		return context.Canceled
	}
	c.cancel = nil
	if err != nil {
		if ws != nil {
			ws.Close()
		}
		c.snapshot = Snapshot{Status: c.snapshot.Status, NeedsStreamStatus: c.snapshot.NeedsStreamStatus, Err: err}
		c.publishLocked()
		return err
	}
	s := &session{ws: ws, endpoint: endpoint, host: u.Host, done: make(chan struct{}), writes: make(chan struct{}, 1), pending: make(map[string]*pending)}
	c.session = s
	c.snapshot = Snapshot{Connected: true, Status: c.snapshot.Status, NeedsStreamStatus: c.snapshot.NeedsStreamStatus}
	c.publishLocked()
	log.Info("obs connected", "elapsed", time.Since(start))
	go c.readLoop(s)
	return nil
}

// setDeadlines 同时设置读写截止时间。
func setDeadlines(ws *websocket.Conn, t time.Time) error {
	if err := ws.SetReadDeadline(t); err != nil {
		return err
	}
	return ws.SetWriteDeadline(t)
}

func identify(ctx context.Context, ws *websocket.Conn, password, host string) error {
	log := logger.With("endpoint", host)
	var e envelope
	if err := ws.ReadJSON(&e); err != nil {
		return transportError(ctx, log, "receive_hello", i18n.T(i18n.OBSReceiveHello), err)
	}
	if e.Op != opHello {
		log.Error("obs handshake failed", "step", "hello", "reason", "unexpected_op", "op_code", e.Op)
		return errors.New(i18n.T(i18n.OBSHelloMissing))
	}
	var hello struct {
		WebSocketVersion string `json:"obsWebSocketVersion"`
		RPCVersion       int    `json:"rpcVersion"`
		Authentication   *struct {
			Challenge string `json:"challenge"`
			Salt      string `json:"salt"`
		} `json:"authentication"`
	}
	if uerr := json.Unmarshal(e.D, &hello); uerr != nil || hello.RPCVersion < rpcVersion {
		log.Error("obs handshake failed", "step", "hello", "reason", "hello_invalid", "rpc_version", hello.RPCVersion, "err", uerr)
		return errors.New(i18n.T(i18n.OBSHelloInvalid))
	}
	log.Info("obs hello received", "ws_version", hello.WebSocketVersion, "rpc_version", hello.RPCVersion, "auth_required", hello.Authentication != nil)
	data := map[string]any{"rpcVersion": rpcVersion, "eventSubscriptions": subscribeOutputs}
	if hello.Authentication != nil {
		if password == "" {
			log.Warn("obs authentication required but no password set")
		}
		if hello.Authentication.Challenge == "" || hello.Authentication.Salt == "" {
			log.Error("obs handshake failed", "step", "authentication", "reason", "challenge_invalid")
			return errors.New(i18n.T(i18n.OBSAuthenticationChallengeInvalid))
		}
		secret := sha256.Sum256([]byte(password + hello.Authentication.Salt))
		proof := sha256.Sum256([]byte(base64.StdEncoding.EncodeToString(secret[:]) + hello.Authentication.Challenge))
		data["authentication"] = base64.StdEncoding.EncodeToString(proof[:])
	}
	if err := writeMessage(ws, opIdentify, data); err != nil {
		return transportError(ctx, log, "send_identify", i18n.T(i18n.OBSIdentify), err)
	}
	if err := ws.ReadJSON(&e); err != nil {
		// 认证失败时 OBS 会以关闭码（如 4009）断开，close_code 由 transportError 记录。
		return transportError(ctx, log, "receive_identified", i18n.T(i18n.OBSReceiveIdentification), err)
	}
	if e.Op != opIdentified {
		log.Error("obs handshake failed", "step", "identify", "reason", "unexpected_op", "op_code", e.Op)
		return errors.New(i18n.T(i18n.OBSIdentificationFailed))
	}
	var identified struct {
		Version int `json:"negotiatedRpcVersion"`
	}
	if uerr := json.Unmarshal(e.D, &identified); uerr != nil || identified.Version != rpcVersion {
		log.Error("obs handshake failed", "step", "identify", "reason", "rpc_version_unsupported", "negotiated_rpc_version", identified.Version, "err", uerr)
		return errors.New(i18n.T(i18n.OBSRpcVersionUnsupported))
	}
	log.Debug("obs identified", "negotiated_rpc_version", identified.Version)
	return nil
}
func writeMessage(ws *websocket.Conn, op int, data any) error {
	return ws.WriteJSON(struct {
		Op int `json:"op"`
		D  any `json:"d"`
	}{op, data})
}

// dropLocked 仅使当前会话失效，并解除所有等待者的阻塞。
func (c *Client) dropLocked(err error) {
	if c.session != nil {
		if err != nil {
			logger.Warn("obs connection dropped", "endpoint", c.session.host, "err", err)
		} else {
			logger.Info("obs disconnected", "endpoint", c.session.host)
		}
		close(c.session.done)
		c.session.ws.Close()
		c.session = nil
	}
	previous := c.snapshot
	c.snapshot = Snapshot{Err: err}
	if err != nil {
		// 传输断开不表示 OBS 已停止；明确 Disconnect 才清除已知状态。
		c.snapshot.Status = previous.Status
		c.snapshot.NeedsStreamStatus = previous.NeedsStreamStatus
	}
	c.publishLocked()
}
func (c *Client) invalidateLocked() {
	c.generation++
	if c.cancel != nil {
		logger.Info("obs connect canceled")
		c.cancel()
		c.cancel = nil
	}
	c.dropLocked(nil)
}
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.invalidateLocked()
	return nil
}
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	logger.Info("obs client closed")
	c.invalidateLocked()
	close(c.events)
	return nil
}
func (c *Client) fail(s *session, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == s {
		c.dropLocked(err)
	}
}

func decodeStatus(data json.RawMessage) (domain.OBSStatus, error) {
	var value struct {
		Active       *bool  `json:"outputActive"`
		Reconnecting *bool  `json:"outputReconnecting"`
		Timecode     string `json:"outputTimecode"`
	}
	if json.Unmarshal(data, &value) != nil || value.Active == nil || value.Reconnecting == nil {
		return domain.OBSStatus{}, errors.New(i18n.T(i18n.OBSStreamStatusInvalid))
	}
	return domain.OBSStatus{Active: *value.Active, Reconnecting: *value.Reconnecting, Timecode: value.Timecode}, nil
}
func (c *Client) readLoop(s *session) {
	log := logger.With("endpoint", s.host)
	for {
		var e envelope
		if rerr := s.ws.ReadJSON(&e); rerr != nil {
			// 主动断开会关闭连接并触发读取错误，此时不是故障。
			c.mu.Lock()
			current := c.session == s
			c.mu.Unlock()
			if !current {
				return
			}
			c.fail(s, transportError(context.Background(), log, "receive_message", i18n.T(i18n.OBSReceiveMessage), rerr))
			return
		}
		if e.Op == opEvent {
			var event struct {
				Type string `json:"eventType"`
				Data struct {
					Active *bool  `json:"outputActive"`
					State  string `json:"outputState"`
				} `json:"eventData"`
			}
			if uerr := json.Unmarshal(e.D, &event); uerr != nil || event.Type == "" {
				log.Error("obs protocol error", "reason", "event_invalid", "err", uerr)
				c.fail(s, errors.New(i18n.T(i18n.OBSEventDataInvalid)))
				return
			}
			if event.Type != "StreamStateChanged" {
				continue
			}
			if event.Data.Active == nil || event.Data.State == "" {
				log.Error("obs protocol error", "reason", "stream_event_invalid", "event", event.Type)
				c.fail(s, errors.New(i18n.T(i18n.OBSStreamEventInvalid)))
				return
			}
			c.mu.Lock()
			if c.session == s {
				c.snapshot.Status.Active = *event.Data.Active
				c.snapshot.Status.Reconnecting = event.Data.State == "OBS_WEBSOCKET_OUTPUT_RECONNECTING"
				if !*event.Data.Active {
					c.snapshot.Status.Timecode = ""
				}
				log.Info("obs stream state changed", "state", event.Data.State, "active", *event.Data.Active, "reconnecting", c.snapshot.Status.Reconnecting)
				c.publishLocked()
			}
			c.mu.Unlock()
			continue
		}
		if e.Op != opRequestResponse {
			log.Error("obs protocol error", "reason", "unexpected_op", "op_code", e.Op)
			c.fail(s, errors.New(i18n.T(i18n.OBSProtocolMessageUnexpected)))
			return
		}
		var response struct {
			Type   string `json:"requestType"`
			ID     string `json:"requestId"`
			Status struct {
				Result  *bool  `json:"result"`
				Code    int    `json:"code"`
				Comment string `json:"comment"`
			} `json:"requestStatus"`
			Data json.RawMessage `json:"responseData"`
		}
		if uerr := json.Unmarshal(e.D, &response); uerr != nil || response.ID == "" || response.Type == "" || response.Status.Result == nil || response.Status.Code == 0 {
			log.Error("obs protocol error", "reason", "response_invalid", "err", uerr)
			c.fail(s, errors.New(i18n.T(i18n.OBSResponseInvalid)))
			return
		}
		c.mu.Lock()
		if c.session != s {
			c.mu.Unlock()
			return
		}
		p := s.pending[response.ID]
		if p != nil {
			result := reply{data: response.Data}
			if response.Type != p.kind {
				log.Error("obs protocol error", "reason", "response_kind_mismatch", "kind", p.kind, "got", response.Type)
				result.err = errors.New(i18n.T(i18n.OBSResponseOperationMismatch))
			} else if !*response.Status.Result || response.Status.Code != requestSucceeded {
				log.Warn("obs request rejected", "kind", p.kind, "code", response.Status.Code, "result", *response.Status.Result, "comment", response.Status.Comment)
				result.err = fmt.Errorf(i18n.T(i18n.OBSRequestRejected), p.kind, response.Status.Code)
			} else if p.kind == "GetStreamStatus" {
				status, err := decodeStatus(response.Data)
				result.err = err
				if err != nil {
					log.Error("obs stream status invalid", "err", err)
				} else {
					prev := c.snapshot.Status
					if prev.Active != status.Active || prev.Reconnecting != status.Reconnecting {
						log.Info("obs stream status changed", "active", status.Active, "reconnecting", status.Reconnecting)
					}
					c.snapshot.Status = status
					// 更早发出的状态查询不能覆盖随后开播请求留下的不确定状态。
					if p.startRevision == c.startRevision {
						c.snapshot.NeedsStreamStatus = false
					}
					c.publishLocked()
				}
			}
			delete(s.pending, response.ID)
			p.result <- result
		}
		c.mu.Unlock()
	}
}

func (c *Client) current() (*session, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session == nil {
		return nil, errors.New(i18n.T(i18n.OBSNotConnected))
	}
	return c.session, nil
}
func (c *Client) request(ctx context.Context, s *session, kind string, data any) (out json.RawMessage, err error) {
	start := time.Now()
	// reported 表示失败原因已在更底层记录过，避免重复。
	reported := false
	defer func() {
		if err == nil {
			logger.Debug("obs request", "kind", kind, "elapsed", time.Since(start))
		} else if !reported {
			logger.Warn("obs request failed", "kind", kind, "elapsed", time.Since(start), "err", err)
		}
	}()
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	select {
	case s.writes <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, errors.New(i18n.T(i18n.OBSDisconnected))
	}
	c.mu.Lock()
	if c.session != s || ctx.Err() != nil {
		c.mu.Unlock()
		<-s.writes
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, errors.New(i18n.T(i18n.OBSDisconnected))
	}
	s.next++
	id := strconv.FormatUint(s.next, 10)
	if kind == "StartStream" {
		c.startRevision++
		c.snapshot.NeedsStreamStatus = true
	}
	p := &pending{kind: kind, result: make(chan reply, 1), startRevision: c.startRevision}
	s.pending[id] = p
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(s.pending, id); c.mu.Unlock() }()
	req := map[string]any{"requestType": kind, "requestId": id}
	if data != nil {
		req["requestData"] = data
	}
	deadline, _ := ctx.Deadline()
	// 取消操作会中断阻塞的写入；超时后的 WebSocket 写入器不可再用。
	stopped := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { s.ws.Close(); close(stopped) })
	werr := s.ws.SetWriteDeadline(deadline)
	if werr == nil {
		werr = writeMessage(s.ws, opRequest, req)
	}
	if !stop() {
		<-stopped
		werr = ctx.Err()
	}
	<-s.writes
	if werr != nil {
		reported = true
		err = transportError(ctx, logger.With("endpoint", s.host, "kind", kind), "send_request", i18n.T(i18n.OBSSendMessage), werr)
		c.fail(s, err)
		return nil, err
	}
	select {
	case result := <-p.result:
		// 响应处理处已记录拒绝/解码失败的详细信息。
		reported = result.err != nil
		return result.data, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.done:
		return nil, errors.New(i18n.T(i18n.OBSDisconnected))
	}
}
func (c *Client) Status(ctx context.Context) (domain.OBSStatus, error) {
	s, err := c.current()
	if err != nil {
		return domain.OBSStatus{}, err
	}
	data, err := c.request(ctx, s, "GetStreamStatus", nil)
	if err != nil {
		return domain.OBSStatus{}, err
	}
	return decodeStatus(data)
}
func (c *Client) operation(ctx context.Context, fn func(context.Context, *session) error) error {
	ctx, cancel := context.WithTimeout(ctx, operationTimeout)
	defer cancel()
	s, err := c.current()
	if err != nil {
		return err
	}
	select {
	case c.operations <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-s.done:
		return errors.New(i18n.T(i18n.OBSDisconnected))
	}
	defer func() { <-c.operations }()
	return fn(ctx, s)
}
func (c *Client) Configure(ctx context.Context, stream domain.Stream) error {
	u, err := url.Parse(stream.Address)
	if err != nil || u.Host == "" || (u.Scheme != "rtmp" && u.Scheme != "rtmps" && u.Scheme != "srt") {
		// 地址可能含敏感路径，仅记录结构特征。
		scheme := ""
		if u != nil {
			scheme = u.Scheme
		}
		logger.Warn("obs stream server invalid", "parse_failed", err != nil, "scheme", scheme)
		return errors.New(i18n.T(i18n.OBSStreamServerInvalid))
	}
	return c.operation(ctx, func(ctx context.Context, s *session) error {
		data, err := c.request(ctx, s, "GetStreamStatus", nil)
		if err != nil {
			return err
		}
		status, err := decodeStatus(data)
		if err != nil {
			logger.Error("obs stream status invalid", "err", err)
			return err
		}
		if status.Active || status.Reconnecting {
			logger.Warn("obs configure refused while streaming", "active", status.Active, "reconnecting", status.Reconnecting)
			return errors.New(i18n.T(i18n.OBSSettingsRequireStoppedStream))
		}
		// 仅记录服务器主机，绝不记录推流密钥。
		logger.Info("obs configure", "server", u.Host, "scheme", u.Scheme)
		// 保留 B 站服务器地址与密钥分离的形式；OBS 自定义服务也支持 SRT。
		_, err = c.request(ctx, s, "SetStreamServiceSettings", map[string]any{
			"streamServiceType":     "rtmp_custom",
			"streamServiceSettings": map[string]any{"bwtest": false, "server": stream.Address, "key": stream.Key, "use_auth": false},
		})
		return err
	})
}
func (c *Client) Start(ctx context.Context) error {
	return c.operation(ctx, func(ctx context.Context, s *session) error {
		_, err := c.request(ctx, s, "StartStream", nil)
		return err
	})
}
func (c *Client) Stop(ctx context.Context) error {
	return c.operation(ctx, func(ctx context.Context, s *session) error {
		_, err := c.request(ctx, s, "StopStream", nil)
		return err
	})
}
