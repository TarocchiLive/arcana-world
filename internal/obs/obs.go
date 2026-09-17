// obs 包实现持久化 OBS WebSocket v5 会话。
package obs

import (
	"arcana-world/internal/i18n"

	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"sync"
	"time"

	"arcana-world/internal/domain"
	"github.com/gorilla/websocket"
)

const operationTimeout = 15 * time.Second

// OBS WebSocket v5 opcodes and negotiated protocol settings.
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
	Connected  bool
	Connecting bool
	Status     domain.OBSStatus
	Err        error
}

type Client struct {
	mu         sync.Mutex
	snapshot   Snapshot
	events     chan Snapshot
	session    *session
	generation uint64
	cancel     context.CancelFunc
	closed     bool
	operations chan struct{}
}

type session struct {
	ws       *websocket.Conn
	done     chan struct{}
	endpoint string
	writes   chan struct{}
	pending  map[string]*pending
	next     uint64
}
type pending struct {
	kind   string
	result chan reply
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
func transportError(ctx context.Context, action string) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf(i18n.T(i18n.OBSOperationContextError), action, err)
	}
	// 服务器关闭原因、URL 和响应说明可能回显机密信息。
	return fmt.Errorf(i18n.T(i18n.OBSTransportFailed), action)
}

func (c *Client) Connect(ctx context.Context, endpoint, password string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || (u.Scheme != "ws" && u.Scheme != "wss") || u.User != nil || u.Fragment != "" {
		return errors.New(i18n.T(i18n.OBSEndpointInvalid))
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return errors.New(i18n.T(i18n.OBSClientClosed))
	}
	if c.snapshot.Connecting {
		c.mu.Unlock()
		return errors.New(i18n.T(i18n.OBSConnectionInProgress))
	}
	// 连接设置仅在明确断开连接后生效。
	if c.session != nil {
		same := c.session.endpoint == endpoint
		c.mu.Unlock()
		if !same {
			return errors.New(i18n.T(i18n.OBSEndpointChangeRequiresDisconnect))
		}
		return nil
	}
	handshake, cancel := context.WithTimeout(ctx, operationTimeout)
	c.generation++
	generation := c.generation
	c.cancel = cancel
	c.snapshot = Snapshot{Connecting: true, Status: c.snapshot.Status}
	c.publishLocked()
	c.mu.Unlock()
	defer cancel()
	ws, response, err := (&websocket.Dialer{HandshakeTimeout: operationTimeout}).DialContext(handshake, endpoint, nil)
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		err = transportError(handshake, i18n.T(i18n.OBSConnect))
	} else {
		stopped := make(chan struct{})
		stop := context.AfterFunc(handshake, func() { ws.Close(); close(stopped) })
		ws.SetReadLimit(1 << 20)
		deadline, _ := handshake.Deadline()
		if ws.SetReadDeadline(deadline) != nil || ws.SetWriteDeadline(deadline) != nil {
			err = transportError(handshake, i18n.T(i18n.OBSSetHandshakeDeadline))
		} else {
			err = identify(handshake, ws, password)
		}
		if !stop() {
			<-stopped
			err = transportError(handshake, i18n.T(i18n.OBSConnect))
		}
		if err == nil && handshake.Err() != nil {
			err = transportError(handshake, i18n.T(i18n.OBSConnect))
		}
		if err == nil && (ws.SetReadDeadline(time.Time{}) != nil || ws.SetWriteDeadline(time.Time{}) != nil) {
			err = transportError(handshake, i18n.T(i18n.OBSFinishHandshake))
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.generation != generation || c.closed {
		if ws != nil {
			ws.Close()
		}
		return context.Canceled
	}
	c.cancel = nil
	if err != nil {
		if ws != nil {
			ws.Close()
		}
		c.snapshot = Snapshot{Status: c.snapshot.Status, Err: err}
		c.publishLocked()
		return err
	}
	s := &session{ws: ws, endpoint: endpoint, done: make(chan struct{}), writes: make(chan struct{}, 1), pending: make(map[string]*pending)}
	c.session = s
	c.snapshot = Snapshot{Connected: true, Status: c.snapshot.Status}
	c.publishLocked()
	go c.readLoop(s)
	return nil
}

func identify(ctx context.Context, ws *websocket.Conn, password string) error {
	var e envelope
	if ws.ReadJSON(&e) != nil {
		return transportError(ctx, i18n.T(i18n.OBSReceiveHello))
	}
	if e.Op != opHello {
		return errors.New(i18n.T(i18n.OBSHelloMissing))
	}
	var hello struct {
		RPCVersion     int `json:"rpcVersion"`
		Authentication *struct {
			Challenge string `json:"challenge"`
			Salt      string `json:"salt"`
		} `json:"authentication"`
	}
	if json.Unmarshal(e.D, &hello) != nil || hello.RPCVersion < rpcVersion {
		return errors.New(i18n.T(i18n.OBSHelloInvalid))
	}
	data := map[string]any{"rpcVersion": rpcVersion, "eventSubscriptions": subscribeOutputs}
	if hello.Authentication != nil {
		if hello.Authentication.Challenge == "" || hello.Authentication.Salt == "" {
			return errors.New(i18n.T(i18n.OBSAuthenticationChallengeInvalid))
		}
		secret := sha256.Sum256([]byte(password + hello.Authentication.Salt))
		proof := sha256.Sum256([]byte(base64.StdEncoding.EncodeToString(secret[:]) + hello.Authentication.Challenge))
		data["authentication"] = base64.StdEncoding.EncodeToString(proof[:])
	}
	if writeMessage(ws, opIdentify, data) != nil {
		return transportError(ctx, i18n.T(i18n.OBSIdentify))
	}
	if ws.ReadJSON(&e) != nil {
		return transportError(ctx, i18n.T(i18n.OBSReceiveIdentification))
	}
	if e.Op != opIdentified {
		return errors.New(i18n.T(i18n.OBSIdentificationFailed))
	}
	var identified struct {
		Version int `json:"negotiatedRpcVersion"`
	}
	if json.Unmarshal(e.D, &identified) != nil || identified.Version != rpcVersion {
		return errors.New(i18n.T(i18n.OBSRpcVersionUnsupported))
	}
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
		close(c.session.done)
		c.session.ws.Close()
		c.session = nil
	}
	status := c.snapshot.Status
	c.snapshot = Snapshot{Err: err}
	if err != nil {
		// 传输断开不表示 OBS 已停止；明确 Disconnect 才清除已知状态。
		c.snapshot.Status = status
	}
	c.publishLocked()
}
func (c *Client) Disconnect() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.generation++
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.dropLocked(nil)
	return nil
}
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	c.generation++
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.dropLocked(nil)
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
	for {
		var e envelope
		if s.ws.ReadJSON(&e) != nil {
			c.fail(s, transportError(context.Background(), i18n.T(i18n.OBSReceiveMessage)))
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
			if json.Unmarshal(e.D, &event) != nil || event.Type == "" {
				c.fail(s, errors.New(i18n.T(i18n.OBSEventDataInvalid)))
				return
			}
			if event.Type != "StreamStateChanged" {
				continue
			}
			if event.Data.Active == nil || event.Data.State == "" {
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
				c.publishLocked()
			}
			c.mu.Unlock()
			continue
		}
		if e.Op != opRequestResponse {
			c.fail(s, errors.New(i18n.T(i18n.OBSProtocolMessageUnexpected)))
			return
		}
		var response struct {
			Type   string `json:"requestType"`
			ID     string `json:"requestId"`
			Status struct {
				Result *bool `json:"result"`
				Code   int   `json:"code"`
			} `json:"requestStatus"`
			Data json.RawMessage `json:"responseData"`
		}
		if json.Unmarshal(e.D, &response) != nil || response.ID == "" || response.Type == "" || response.Status.Result == nil || response.Status.Code == 0 {
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
				result.err = errors.New(i18n.T(i18n.OBSResponseOperationMismatch))
			} else if !*response.Status.Result || response.Status.Code != requestSucceeded {
				result.err = fmt.Errorf(i18n.T(i18n.OBSRequestRejected), p.kind, response.Status.Code)
			} else if p.kind == "GetStreamStatus" {
				status, err := decodeStatus(response.Data)
				result.err = err
				if err == nil {
					c.snapshot.Status = status
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
func (c *Client) request(ctx context.Context, s *session, kind string, data any) (json.RawMessage, error) {
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
	p := &pending{kind: kind, result: make(chan reply, 1)}
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
	err := s.ws.SetWriteDeadline(deadline)
	if err == nil {
		err = writeMessage(s.ws, opRequest, req)
	}
	if !stop() {
		<-stopped
		err = ctx.Err()
	}
	<-s.writes
	if err != nil {
		err = transportError(ctx, i18n.T(i18n.OBSSendMessage))
		c.fail(s, err)
		return nil, err
	}
	select {
	case result := <-p.result:
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
		return errors.New(i18n.T(i18n.OBSStreamServerInvalid))
	}
	return c.operation(ctx, func(ctx context.Context, s *session) error {
		data, err := c.request(ctx, s, "GetStreamStatus", nil)
		if err != nil {
			return err
		}
		status, err := decodeStatus(data)
		if err != nil {
			return err
		}
		if status.Active || status.Reconnecting {
			return errors.New(i18n.T(i18n.OBSSettingsRequireStoppedStream))
		}
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
