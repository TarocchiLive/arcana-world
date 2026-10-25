package tts

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"

	"arcana-world/internal/i18n"
	"arcana-world/internal/tts/edgetts"
	"github.com/gorilla/websocket"
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
	proxy    func(*http.Request) (*url.URL, error)
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
	return &cloudSession{service: service, client: client, proxy: client.Transport.(*http.Transport).Proxy, lifetime: lifetime, cancel: cancel}, nil
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
		started := time.Now()
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
		logger.Warn("online synthesis attempt failed", "service", s.service, "attempt", i+1, "attempts", len(cloudRetryDelays), "elapsed", time.Since(started), "timeout", timedOut, "err", err)
		if timedOut {
			err = retryableError{s.fail(i18n.TTSCloudTimeout)}
		}
		var retry retryableError
		if !errors.As(err, &retry) {
			return nil, err
		}
		last = retry.error
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

// requestHTTP 记录传输故障与非 200 状态；path 由调用方给出固定协议路由或占位符，签名 URL 不进入日志。
func (s *cloudSession) requestHTTP(req *http.Request, stage, path string) (*http.Response, error) {
	started := time.Now()
	response, err := s.client.Do(req)
	status := 0
	if response != nil {
		status = response.StatusCode
	}
	if err != nil || status != http.StatusOK {
		s.logDiagnostic(req.Context(), 3, stage, err, "host", req.URL.Hostname(), "path", path, "status", status, "elapsed", time.Since(started))
	}
	return response, err
}

// diagnostic 在失败转换为用户提示之前记录技术原因，attrs 追加阶段相关字段。
func (s *cloudSession) diagnostic(ctx context.Context, stage string, err error, attrs ...any) {
	s.logDiagnostic(ctx, 3, stage, err, attrs...)
}

// logDiagnostic 以跳过 skip 层后的调用位置作为 source，便于定位产生失败的代码。
// 调用方取消或合成器关闭不属于故障，不记录。
func (s *cloudSession) logDiagnostic(ctx context.Context, skip int, stage string, err error, attrs ...any) {
	if errors.Is(ctx.Err(), context.Canceled) || s.lifetime.Err() != nil || !logger.Enabled(ctx, slog.LevelWarn) {
		return
	}
	var pc [1]uintptr
	runtime.Callers(skip, pc[:])
	record := slog.NewRecord(time.Now(), slog.LevelWarn, "online synthesis failed", pc[0])
	record.Add("service", s.service, "stage", stage)
	if err != nil {
		record.Add("cause", safeCloudCause(err))
	}
	record.Add(attrs...)
	_ = logger.Handler().Handle(context.Background(), record)
}

// safeCloudCause 只输出可安全记录的错误类别与解码偏移，不含 URL、响应内容、服务端自由文本或语音文本。
func safeCloudCause(err error) string {
	var rejected *edgetts.HandshakeError
	var syntax *json.SyntaxError
	var mismatch *json.UnmarshalTypeError
	var timeout net.Error
	var dns *net.DNSError
	var verify *tls.CertificateVerificationError
	var closeErr *websocket.CloseError
	var op *net.OpError
	var errno syscall.Errno
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout():
		return "timeout"
	case errors.As(err, &rejected):
		return "handshake rejected (HTTP " + strconv.Itoa(rejected.Status) + ")"
	case errors.Is(err, edgetts.ErrEmptyAudio):
		return "empty audio"
	case errors.Is(err, edgetts.ErrAudioTooLarge):
		return "audio too large"
	case errors.Is(err, edgetts.ErrProtocol):
		return "protocol error"
	case errors.As(err, &syntax):
		return fmt.Sprintf("JSON syntax error at byte %d", syntax.Offset)
	case errors.As(err, &mismatch):
		return fmt.Sprintf("JSON type mismatch at byte %d", mismatch.Offset)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return "unexpected EOF"
	case errors.Is(err, io.EOF):
		return "EOF"
	case errors.As(err, &dns):
		return "dns lookup failed: " + dns.Err
	case errors.As(err, &verify):
		return "tls certificate verification failed: " + verify.Err.Error()
	case errors.As(err, &closeErr):
		return "websocket closed (code " + strconv.Itoa(closeErr.Code) + ")"
	case errors.As(err, &op) && errors.As(err, &errno):
		return op.Op + ": " + errno.Error()
	case errors.As(err, &op):
		return op.Op + " failed"
	}
	// 未归类的错误只记录最内层类型，外层包装可能携带 URL。
	for inner := errors.Unwrap(err); inner != nil; inner = errors.Unwrap(err) {
		err = inner
	}
	return fmt.Sprintf("unclassified %T", err)
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
