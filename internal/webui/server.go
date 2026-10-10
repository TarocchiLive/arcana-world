package webui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"arcana-world/internal/i18n"
)

//go:embed static/*
var assets embed.FS

// Server 仅绑定 IPv4 回环地址。每次启动换用随机凭证，不写入磁盘。
type Server struct {
	http     *http.Server
	listener net.Listener
	token    string
	host     string
	request  Request
}

func Listen(port int, request Request) (*Server, error) {
	if port < 0 || port > 65535 {
		return nil, errors.New(i18n.T(i18n.WebPortInvalid))
	}
	var secret [32]byte
	if _, err := rand.Read(secret[:]); err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		return nil, err
	}
	s := &Server{listener: listener, token: base64.RawURLEncoding.EncodeToString(secret[:]), host: listener.Addr().String(), request: request}
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

func (s *Server) URL() string {
	return "http://" + strings.TrimSuffix(s.host, ":80") + "/#token=" + s.token
}

func (s *Server) Serve() error {
	err := s.http.Serve(s.listener)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func (s *Server) Close(ctx context.Context) error {
	err := s.http.Shutdown(ctx)
	if err != nil {
		return errors.Join(err, s.http.Close())
	}
	return nil
}

func fail(w http.ResponseWriter, code int, key i18n.Key) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": i18n.T(key)})
}

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
	w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data: https:; connect-src 'self'; font-src 'self'; base-uri 'none'; form-action 'none'; frame-ancestors 'none'")
	// 严格匹配启动地址，拒绝 DNS 重绑定及其他站点对本机服务的访问。
	if r.Host != s.host && r.Host != strings.TrimSuffix(s.host, ":80") {
		fail(w, http.StatusForbidden, i18n.WebOriginDenied)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && origin != "http://"+s.host && origin != "http://"+strings.TrimSuffix(s.host, ":80") {
		fail(w, http.StatusForbidden, i18n.WebOriginDenied)
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		fail(w, http.StatusForbidden, i18n.WebOriginDenied)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(provided), []byte(s.token)) != 1 {
			fail(w, http.StatusUnauthorized, i18n.WebUnauthorized)
			return
		}
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
	case "/api/command":
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			fail(w, http.StatusMethodNotAllowed, i18n.WebMethodInvalid)
			return
		}
		if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
			fail(w, http.StatusUnsupportedMediaType, i18n.WebJSONRequired)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
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
