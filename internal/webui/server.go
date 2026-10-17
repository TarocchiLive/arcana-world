package webui

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"arcana-world/internal/broadcast"
	"arcana-world/internal/i18n"
)

//go:embed static/*
var assets embed.FS

// Server 默认仅绑定 IPv4 回环地址；全接口监听必须通过配置显式启用。
type Server struct {
	http        *http.Server
	listener    net.Listener
	bindAddress string
	host        string
	request     Request
	mu          sync.Mutex
	broadcast   *broadcast.Server
	closed      bool
	closeOnce   sync.Once
	closeErr    error
}

func Listen(bindAddress string, port int, request Request) (*Server, error) {
	if bindAddress == "" {
		bindAddress = "127.0.0.1"
	}
	if bindAddress != "127.0.0.1" && bindAddress != "0.0.0.0" {
		return nil, errors.New(i18n.T(i18n.WebBindInvalid))
	}
	if port < 0 || port > 65535 {
		return nil, errors.New(i18n.T(i18n.WebPortInvalid))
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(bindAddress, strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	// 启动链接始终使用本机可访问地址，不把通配监听地址当作浏览器目的地址。
	host := net.JoinHostPort("127.0.0.1", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))
	s := &Server{listener: listener, bindAddress: bindAddress, host: host, request: request}
	s.http = &http.Server{
		Handler:           http.HandlerFunc(s.serveHTTP),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    8 << 10,
	}
	return s, nil
}

// Addr 返回实际绑定地址，保留通配地址及系统分配的端口。
func (s *Server) Addr() string {
	return s.listener.Addr().String()
}

// SetBroadcast 替换只读广播；旧广播由调用方在配置提交后关闭。
// WebUI 关闭后不再接受挂载，传入的广播也会立即关闭。
func (s *Server) SetBroadcast(server *broadcast.Server) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		if server != nil {
			_ = server.Close()
		}
		return
	}
	s.broadcast = server
}

func (s *Server) URL() string {
	return "http://" + strings.TrimSuffix(s.host, ":80") + "/"
}

func (s *Server) Serve() error {
	err := s.http.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Close(ctx context.Context) error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		if s.broadcast != nil {
			s.closeErr = s.broadcast.Close()
			s.broadcast = nil
		}
		s.mu.Unlock()
		err := s.http.Shutdown(ctx)
		if err != nil {
			err = errors.Join(err, s.http.Close())
		}
		// Serve 尚未启动时，HTTP 服务还未接管监听器，也必须释放端口。
		listenerErr := s.listener.Close()
		if errors.Is(listenerErr, net.ErrClosed) {
			listenerErr = nil
		}
		s.closeErr = errors.Join(s.closeErr, err, listenerErr)
	})
	return s.closeErr
}

func fail(w http.ResponseWriter, code int, key i18n.Key) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": i18n.T(key)})
}

func (s *Server) validHost(r *http.Request) bool {
	if s.bindAddress == "127.0.0.1" {
		return r.Host == s.host || r.Host == strings.TrimSuffix(s.host, ":80")
	}
	// 全接口监听也只接受当前连接实际到达的 IPv4 地址，拒绝域名及伪造目的地址。
	local, ok := r.Context().Value(http.LocalAddrContextKey).(*net.TCPAddr)
	if !ok || local.IP.To4() == nil {
		return false
	}
	host := r.Host
	if local.Port == 80 && !strings.Contains(host, ":") {
		host += ":80"
	}
	address, err := netip.ParseAddrPort(host)
	return err == nil && address.Addr().Is4() &&
		int(address.Port()) == local.Port &&
		local.Port == s.listener.Addr().(*net.TCPAddr).Port &&
		address.Addr().String() == local.IP.String() &&
		host == address.String()
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	// 只有已挂载的精确 /ws 路径绕过 WebUI 的 Host、Origin 与来源限制。
	if r.URL.Path == "/ws" {
		s.mu.Lock()
		server := s.broadcast
		s.mu.Unlock()
		if server != nil {
			server.ServeHTTP(w, r)
			return
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob: https:; connect-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	// 严格匹配连接地址，拒绝 DNS 重绑定及其他站点对服务的访问。
	if !s.validHost(r) {
		fail(w, http.StatusForbidden, i18n.WebOriginDenied)
		return
	}
	origin := r.Header.Get("Origin")
	sameOrigin := origin == "http://"+r.Host
	if s.listener.Addr().(*net.TCPAddr).Port == 80 {
		host := strings.TrimSuffix(r.Host, ":80")
		sameOrigin = origin == "http://"+host || origin == "http://"+host+":80"
	}
	if origin != "" && !sameOrigin {
		fail(w, http.StatusForbidden, i18n.WebOriginDenied)
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		fail(w, http.StatusForbidden, i18n.WebOriginDenied)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.api(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		fail(w, http.StatusMethodNotAllowed, i18n.WebMethodInvalid)
		return
	}
	// 不提供目录列表、源码映射或任意嵌入文件路径。
	name := ""
	switch r.URL.Path {
	case "/":
		name = "index.html"
	case "/app.css":
		name = "app.css"
	case "/app.js":
		name = "app.js"
	default:
		http.NotFound(w, r)
		return
	}
	static, _ := fs.Sub(assets, "static")
	http.ServeFileFS(w, r, static, name)
}

func (s *Server) api(w http.ResponseWriter, r *http.Request) {
	var command *Command
	page := r.URL.Query().Get("page")
	switch r.URL.Path {
	case "/api/state":
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			fail(w, http.StatusMethodNotAllowed, i18n.WebMethodInvalid)
			return
		}
	case "/api/command", "/api/cover":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			fail(w, http.StatusMethodNotAllowed, i18n.WebMethodInvalid)
			return
		}
		if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
			fail(w, http.StatusUnsupportedMediaType, i18n.WebJSONRequired)
			return
		}
		bodyLimit := int64(64 << 10)
		if r.URL.Path == "/api/cover" {
			bodyLimit = 2 << 20
		}
		r.Body = http.MaxBytesReader(w, r.Body, bodyLimit)
		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		command = &Command{}
		if err := decoder.Decode(command); err != nil {
			fail(w, http.StatusBadRequest, i18n.WebRequestInvalid)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			fail(w, http.StatusBadRequest, i18n.WebRequestInvalid)
			return
		}
		// 图片接口只能准备封面，普通命令接口不能绕过图片请求的边界。
		if (r.URL.Path == "/api/cover" && (command.Page != "studio" || command.Action != "cover-upload")) ||
			(r.URL.Path == "/api/command" && command.Action == "cover-upload") {
			fail(w, http.StatusBadRequest, i18n.WebRequestInvalid)
			return
		}
		page = command.Page
	default:
		http.NotFound(w, r)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	state, err := s.request(ctx, page, command)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			fail(w, http.StatusServiceUnavailable, i18n.WebUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprint(err)})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(state)
}
