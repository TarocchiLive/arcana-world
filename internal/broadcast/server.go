// Package broadcast 提供只读的弹幕、礼物与观众互动实时广播。
package broadcast

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type client struct {
	conn  *websocket.Conn
	queue chan []byte
}

// Server 不保存历史；慢客户端断开后需自行重新连接。
type Server struct {
	mu        sync.Mutex
	clients   map[*client]struct{}
	http      *http.Server
	ctx       context.Context
	cancel    context.CancelFunc
	done      chan struct{}
	err       error
	closeOnce sync.Once
	closeErr  error
}

// New 创建不占用监听地址的广播服务，可挂载到现有 HTTP 服务。
func New() *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{clients: make(map[*client]struct{}), ctx: ctx, cancel: cancel}
}

// Listen 同步绑定地址，绑定失败时不启动后台服务。
func Listen(addr string) (*Server, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	s := New()
	s.done = make(chan struct{})
	s.http = &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		defer close(s.done)
		s.err = s.http.Serve(listener)
	}()
	return s, nil
}

// ServeHTTP 仅提供精确匹配的 /ws 只读广播端点。
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/ws" {
		http.NotFound(w, r)
		return
	}
	if s.ctx.Err() != nil {
		http.Error(w, "broadcast closed", http.StatusServiceUnavailable)
		return
	}
	s.accept(w, r)
}

func (s *Server) accept(w http.ResponseWriter, r *http.Request) {
	// 广播允许 OBS 浏览器源和其他跨源网页连接，不校验 Origin。
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	c := &client{conn: conn, queue: make(chan []byte, 64)}
	s.mu.Lock()
	if s.ctx.Err() != nil {
		s.mu.Unlock()
		_ = conn.CloseNow()
		return
	}
	s.clients[c] = struct{}{}
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
		_ = conn.CloseNow()
	}()
	// 只读连接仍需消费控制帧，以发现断线并回应 ping。
	ctx := conn.CloseRead(s.ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case data := <-c.queue:
			writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(writeCtx, websocket.MessageText, data)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

// Publish 保留原始字段，仅转发弹幕、礼物与观众互动消息；不等待客户端网络写入。
func (s *Server) Publish(room int64, raw json.RawMessage) {
	var command struct {
		Cmd string `json:"cmd"`
	}
	if json.Unmarshal(raw, &command) != nil {
		return
	}
	switch strings.SplitN(command.Cmd, ":", 2)[0] {
	case "DANMU_MSG", "SEND_GIFT", "INTERACT_WORD", "INTERACT_WORD_V2", "LIKE_INFO_V3_CLICK":
	default:
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.clients) == 0 || s.ctx.Err() != nil {
		return
	}
	data, err := json.Marshal(struct {
		RoomID int64           `json:"room_id"`
		Event  json.RawMessage `json:"event"`
	}{room, raw})
	if err != nil {
		return
	}
	for c := range s.clients {
		select {
		case c.queue <- data:
		default:
			delete(s.clients, c)
			_ = c.conn.CloseNow()
		}
	}
}

// Close 停止独立监听及已升级的连接；挂载模式不关闭外部 HTTP 服务。
func (s *Server) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		if s.http != nil {
			s.closeErr = s.http.Close()
		}
		s.mu.Lock()
		for c := range s.clients {
			_ = c.conn.CloseNow()
		}
		s.mu.Unlock()
		if s.done != nil {
			<-s.done
			if s.err != http.ErrServerClosed {
				s.closeErr = s.err
			}
		}
	})
	return s.closeErr
}
