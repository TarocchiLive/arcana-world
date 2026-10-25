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
	"sync/atomic"
	"time"

	"arcana-world/internal/i18n"
	"arcana-world/internal/journal"
)

//go:embed static/*
var assets embed.FS

var logger = journal.Logger("webui")

// Server 默认仅绑定 IPv4 回环地址；全接口监听必须通过配置显式启用。
type Server struct {
	http        *http.Server
	listener    net.Listener
	bindAddress string
	host        string
	request     Request
	clients     atomic.Int64 // 当前打开的 TCP 连接数。
}

func Listen(bindAddress string, port int, request Request) (*Server, error) {
	if bindAddress == "" {
		bindAddress = "127.0.0.1"
	}
	if bindAddress != "127.0.0.1" && bindAddress != "0.0.0.0" {
		logger.Error("web ui listen rejected", "reason", "invalid bind address", "bind", bindAddress, "port", port)
		return nil, errors.New(i18n.T(i18n.WebBindInvalid))
	}
	if port < 0 || port > 65535 {
		logger.Error("web ui listen rejected", "reason", "invalid port", "bind", bindAddress, "port", port)
		return nil, errors.New(i18n.T(i18n.WebPortInvalid))
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort(bindAddress, strconv.Itoa(port)))
	if err != nil {
		logger.Error("web ui listen failed", "bind", bindAddress, "port", port, "err", err)
		return nil, fmt.Errorf(i18n.T(i18n.WebListenFailed), port)
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
		ConnState:         s.trackConn,
	}
	logger.Info("web ui listening", "addr", listener.Addr().String(), "bind", bindAddress, "url", s.URL())
	return s, nil
}

func (s *Server) URL() string {
	return "http://" + strings.TrimSuffix(s.host, ":80") + "/"
}

func (s *Server) Serve() error {
	err := s.http.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	logger.Error("web ui serve failed", "addr", s.listener.Addr().String(), "err", err)
	return errors.New(i18n.T(i18n.WebServeFailed))
}

func (s *Server) Close(ctx context.Context) error {
	start := time.Now()
	err := s.http.Shutdown(ctx)
	if err != nil {
		closeErr := s.http.Close()
		logger.Warn("web ui shutdown forced", "elapsed", time.Since(start), "clients", s.clients.Load(), "err", err, "close_err", closeErr)
		return errors.Join(err, closeErr)
	}
	logger.Info("web ui shutdown", "addr", s.listener.Addr().String(), "elapsed", time.Since(start))
	return nil
}

// trackConn 记录客户端连接的建立与断开；只在连接层记录，不记录每个请求。
func (s *Server) trackConn(c net.Conn, state http.ConnState) {
	switch state {
	case http.StateNew:
		logger.Debug("web client connected", "remote", c.RemoteAddr().String(), "clients", s.clients.Add(1))
	case http.StateClosed, http.StateHijacked:
		logger.Debug("web client disconnected", "remote", c.RemoteAddr().String(), "clients", s.clients.Add(-1))
	}
}

// reject 记录被拒绝的请求及原因，再向客户端返回本地化错误。
func reject(w http.ResponseWriter, r *http.Request, code int, key i18n.Key, reason string) {
	logger.Warn("web request rejected", "reason", reason, "method", r.Method, "path", r.URL.Path, "status", code,
		"remote", r.RemoteAddr, "host", r.Host, "origin", r.Header.Get("Origin"), "sec_fetch_site", r.Header.Get("Sec-Fetch-Site"))
	fail(w, code, key)
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
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: blob: https:; connect-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	// 严格匹配连接地址，拒绝 DNS 重绑定及其他站点对服务的访问。
	if !s.validHost(r) {
		reject(w, r, http.StatusForbidden, i18n.WebOriginDenied, "host mismatch")
		return
	}
	origin := r.Header.Get("Origin")
	sameOrigin := origin == "http://"+r.Host
	if s.listener.Addr().(*net.TCPAddr).Port == 80 {
		host := strings.TrimSuffix(r.Host, ":80")
		sameOrigin = origin == "http://"+host || origin == "http://"+host+":80"
	}
	if origin != "" && !sameOrigin {
		reject(w, r, http.StatusForbidden, i18n.WebOriginDenied, "origin mismatch")
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		reject(w, r, http.StatusForbidden, i18n.WebOriginDenied, "cross-site fetch")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		s.api(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		reject(w, r, http.StatusMethodNotAllowed, i18n.WebMethodInvalid, "method not allowed")
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
			reject(w, r, http.StatusMethodNotAllowed, i18n.WebMethodInvalid, "method not allowed")
			return
		}
	case "/api/command", "/api/cover":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			reject(w, r, http.StatusMethodNotAllowed, i18n.WebMethodInvalid, "method not allowed")
			return
		}
		if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
			reject(w, r, http.StatusUnsupportedMediaType, i18n.WebJSONRequired, "content type not json")
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
			logger.Warn("web request body invalid", "method", r.Method, "path", r.URL.Path, "status", http.StatusBadRequest, "remote", r.RemoteAddr, "err", err)
			fail(w, http.StatusBadRequest, i18n.WebRequestInvalid)
			return
		}
		if err := decoder.Decode(new(any)); err != io.EOF {
			logger.Warn("web request body invalid", "method", r.Method, "path", r.URL.Path, "status", http.StatusBadRequest, "remote", r.RemoteAddr, "err", errors.New("trailing data after json value"))
			fail(w, http.StatusBadRequest, i18n.WebRequestInvalid)
			return
		}
		// 图片接口只能准备封面，普通命令接口不能绕过图片请求的边界。
		if (r.URL.Path == "/api/cover" && (command.Page != "studio" || command.Action != "cover-upload")) ||
			(r.URL.Path == "/api/command" && command.Action == "cover-upload") {
			reject(w, r, http.StatusBadRequest, i18n.WebRequestInvalid, "action not allowed on endpoint")
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
			logger.Warn("web request unavailable", "method", r.Method, "path", r.URL.Path, "page", page, "status", http.StatusServiceUnavailable, "err", err)
			fail(w, http.StatusServiceUnavailable, i18n.WebUnavailable)
			return
		}
		logger.Warn("web request failed", "method", r.Method, "path", r.URL.Path, "page", page, "status", http.StatusConflict, "err", err)
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": fmt.Sprint(err)})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(state)
}
