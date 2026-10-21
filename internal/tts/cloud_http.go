package tts

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"arcana-world/internal/i18n"
)

// cloudRetryDelays 是在线合成每次尝试前的等待时间：失败后隔 0.5 秒、1.5 秒各重试一次。
var cloudRetryDelays = [...]time.Duration{0, 500 * time.Millisecond, 1500 * time.Millisecond}

// retryableError 标记换一次请求可能成功的失败：网络中断、超时、限流、服务端故障或空音频。
type retryableError struct{ error }

func (e retryableError) Unwrap() error { return e.error }

// cloudSession 为每个在线合成器独立管理连接、重试和关闭时的取消。
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

// synthesize 登记一次合成并按 cloudRetryDelays 重试可恢复的失败；每次尝试单独计时。
// Close 或调用方取消会立即结束等待和进行中的请求。
func (s *cloudSession) synthesize(ctx context.Context, attempt func(context.Context) ([]byte, error)) ([]byte, error) {
	ctx, done, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	var last error
	for i, delay := range cloudRetryDelays {
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, s.stopped(ctx)
			}
		}
		attemptCtx, cancel := context.WithTimeout(ctx, defaultSynthesisTimeout)
		audio, err := attempt(attemptCtx)
		timedOut := errors.Is(attemptCtx.Err(), context.DeadlineExceeded)
		cancel()
		if err == nil {
			return audio, nil
		}
		if ctx.Err() != nil {
			return nil, s.stopped(ctx)
		}
		if timedOut {
			err = retryableError{s.fail(i18n.TTSCloudTimeout)}
		}
		var retry retryableError
		if !errors.As(err, &retry) {
			return nil, err
		}
		last = retry.error
		logger.Warn("online synthesis attempt failed", "service", s.service, "attempt", i+1, "attempts", len(cloudRetryDelays), "err", last)
	}
	return nil, last
}

// begin 登记一次合成；返回的上下文在 Close 时取消，done 解除登记。
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
	ctx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.lifetime, cancel)
	done := func() {
		stop()
		cancel()
		s.requests.Done()
	}
	// Close 与注册回调并发时，也不能启动新的请求。
	if s.lifetime.Err() != nil {
		done()
		return nil, nil, ErrClosed
	}
	return ctx, done, nil
}

// stopped 返回合成被中止的原因：合成器已关闭，或调用方取消。
func (s *cloudSession) stopped(ctx context.Context) error {
	if s.lifetime.Err() != nil {
		return ErrClosed
	}
	return ctx.Err()
}

// fail 生成带服务名称的用户提示。
func (s *cloudSession) fail(key i18n.Key) error {
	return fmt.Errorf(i18n.T(key), s.service)
}

// failure 报告可重试的网络失败；不回传底层错误，其中可能包含 API 地址、签名音频 URL 或代理凭据。
// 上下文已结束时返回其原因，由 synthesize 区分单次超时与取消。
func (s *cloudSession) failure(ctx context.Context, key i18n.Key) error {
	if s.lifetime.Err() != nil {
		return ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return retryableError{s.fail(key)}
}

// Close 先取消再等待，返回后不存在进行中的请求。
func (s *cloudSession) Close() error {
	s.mu.Lock()
	s.closed = true
	s.cancel()
	s.mu.Unlock()
	s.requests.Wait()
	s.client.CloseIdleConnections()
	return nil
}

// statusError 把服务端状态码转换为用户可以直接处理的提示，限流和服务端故障可以重试；
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
		return retryableError{s.fail(i18n.TTSStatusRateLimited)}
	case status >= http.StatusInternalServerError:
		return retryableError{s.fail(i18n.TTSStatusUnavailable)}
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
