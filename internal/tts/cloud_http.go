package tts

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"

	"arcana-world/internal/i18n"
)

// cloudSession 为每个云端合成器独立管理连接、请求超时和关闭时的取消。
type cloudSession struct {
	service  string // 写入用户提示的服务名称。
	client   *http.Client
	lifetime context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	closed   bool
	requests sync.WaitGroup
}

func newCloudSession(service, proxy string) (*cloudSession, error) {
	client, err := newCloudHTTP(proxy)
	if err != nil {
		return nil, err
	}
	lifetime, cancel := context.WithCancel(context.Background())
	return &cloudSession{service: service, client: client, lifetime: lifetime, cancel: cancel}, nil
}

// begin 登记一次请求并返回带超时的上下文；Close 会取消该上下文并等待 done。
func (s *cloudSession) begin(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		return nil, nil, errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil, nil, ErrClosed
	}
	s.requests.Add(1)
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, defaultSynthesisTimeout)
	stop := context.AfterFunc(s.lifetime, cancel)
	done := func() {
		stop()
		cancel()
		s.requests.Done()
	}
	// Close 与注册回调并发时，也不能启动新的付费请求。
	if s.lifetime.Err() != nil {
		done()
		return nil, nil, ErrClosed
	}
	return ctx, done, nil
}

// fail 生成带服务名称的用户提示。
func (s *cloudSession) fail(key i18n.Key) error {
	return fmt.Errorf(i18n.T(key), s.service)
}

// failure 不回传底层错误，其中可能包含 API 地址、签名音频 URL 或代理凭据。
func (s *cloudSession) failure(ctx context.Context, key i18n.Key) error {
	if s.lifetime.Err() != nil {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return s.fail(key)
}

// Close 先取消再等待，禁止返回后仍存在进行中的付费请求。
func (s *cloudSession) Close() error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.requests.Wait()
	s.client.CloseIdleConnections()
	return nil
}

// statusError 把服务端状态码转换为用户可以直接处理的提示；
// 不读取响应体，避免服务端回显的内容进入界面或日志。
func (s *cloudSession) statusError(status int) error {
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return s.fail(i18n.TTSStatusUnauthorized)
	case status == http.StatusNotFound:
		return s.fail(i18n.TTSStatusNotFound)
	case status == http.StatusBadRequest || status == http.StatusUnprocessableEntity:
		return s.fail(i18n.TTSStatusBadRequest)
	case status == http.StatusTooManyRequests:
		return s.fail(i18n.TTSStatusRateLimited)
	case status >= http.StatusInternalServerError:
		return s.fail(i18n.TTSStatusUnavailable)
	}
	return fmt.Errorf(i18n.T(i18n.TTSStatusUnexpected), s.service, status)
}

// newCloudHTTP 为每个合成器独立管理连接；禁止重定向，避免凭据跨站泄漏。
func newCloudHTTP(proxy string) (*http.Client, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	switch proxy {
	case "direct":
		transport.Proxy = nil
	case "":
		// 按实际请求地址解析环境代理，保留 NO_PROXY 语义。
		transport.Proxy = http.ProxyFromEnvironment
	default:
		normalized, err := normalizeProxy(proxy)
		if err != nil {
			return nil, err
		}
		parsed, err := url.Parse(normalized)
		if err != nil {
			return nil, errors.New(i18n.T(i18n.TTSProxyInvalid))
		}
		transport.Proxy = http.ProxyURL(parsed)
	}
	transport.MaxResponseHeaderBytes = 64 << 10
	transport.ResponseHeaderTimeout = defaultSynthesisTimeout
	return &http.Client{
		Transport: transport,
		Timeout:   defaultSynthesisTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}, nil
}
