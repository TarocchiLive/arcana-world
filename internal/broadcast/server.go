// Package broadcast 提供只读的直播间事件实时广播。
package broadcast

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type client struct {
	conn  *websocket.Conn
	queue chan []byte
}

// Server 不保存历史；慢客户端断开后需自行重新连接。
type Server struct {
	mu      sync.Mutex
	clients map[*client]struct{}
	token   string
	http    *http.Server
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
	err     error
}

// Listen 同步绑定地址，绑定失败时不启动后台服务；token 为空表示不验证。
func Listen(addr, token string) (*Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{clients: make(map[*client]struct{}), token: token, ctx: ctx, cancel: cancel, done: make(chan struct{})}
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", s.accept)
	s.http = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		defer close(s.done)
		s.err = s.http.Serve(listener)
	}()
	return s, nil
}

// SetToken 更换令牌并断开现有连接，客户端需用新令牌重连。
func (s *Server) SetToken(token string) {
	s.mu.Lock()
	if s.token == token {
		s.mu.Unlock()
		return
	}
	s.token = token
	clients := s.clients
	s.clients = make(map[*client]struct{})
	s.mu.Unlock()
	for c := range clients {
		_ = c.conn.Close()
	}
}

// authorized 接受查询参数 token 或 Authorization: Bearer；浏览器 WebSocket 无法自定义请求头。
func authorized(r *http.Request, token string) bool {
	if token == "" {
		return true
	}
	got := r.URL.Query().Get("token")
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		got = bearer
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(token)) == 1
}

func (s *Server) accept(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	token := s.token
	s.mu.Unlock()
	if !authorized(r, token) {
		http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
		return
	}
	// 广播允许 OBS 浏览器源和其他跨源网页连接，不校验 Origin。
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	c := &client{conn: conn, queue: make(chan []byte, 64)}
	s.mu.Lock()
	// 升级期间可能更换令牌，注册连接前在锁内重新验证。
	if s.ctx.Err() != nil || !authorized(r, s.token) {
		s.mu.Unlock()
		_ = conn.Close()
		return
	}
	s.clients[c] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		_ = conn.Close()
	}()
	// 只读连接仍需消费控制帧，以发现断线并回应 ping。
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer conn.Close()
		if _, _, err := conn.NextReader(); err == nil {
			_ = conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "unexpected data message"),
				time.Now().Add(5*time.Second))
		}
	}()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-done:
			return
		case data := <-c.queue:
			if err := conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
				return
			}
		}
	}
}

// Connected 报告是否有客户端，调用方据此跳过事件解析。
func (s *Server) Connected() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.clients) != 0
}

// Prepare 在调用方锁外封装事件；没有客户端时不分配消息。raw 必须是有效 JSON。
// 返回的提交函数只进行非阻塞入队，不解析、序列化或进行网络 I/O。
func (s *Server) Prepare(room int64, raw json.RawMessage) func() {
	s.mu.Lock()
	active := len(s.clients) != 0 && s.ctx.Err() == nil
	s.mu.Unlock()
	if !active {
		return nil
	}
	data := make([]byte, 0, len(raw)+40)
	data = append(data, `{"room_id":`...)
	data = strconv.AppendInt(data, room, 10)
	data = append(data, `,"event":`...)
	data = append(data, raw...)
	data = append(data, '}')
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.ctx.Err() != nil {
			return
		}
		for c := range s.clients {
			select {
			case c.queue <- data:
			default:
				delete(s.clients, c)
				// 每个慢客户端只移除一次，异步关闭以免阻塞入队。
				go c.conn.Close()
			}
		}
	}
}

// Close 同时停止 HTTP 监听和已升级的连接。
func (s *Server) Close() error {
	s.cancel()
	err := s.http.Close()
	s.mu.Lock()
	clients := s.clients
	s.clients = nil
	s.mu.Unlock()
	for c := range clients {
		_ = c.conn.Close()
	}
	<-s.done
	if s.err != http.ErrServerClosed {
		return s.err
	}
	return err
}
