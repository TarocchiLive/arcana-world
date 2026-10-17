package tts

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/gorilla/websocket"
	edge "github.com/wujunwei928/edge-tts-go/edge_tts"
)

const edgeReceiveTimeoutSeconds = 20

// RunEdge 在专用进程中处理一个请求。它会修改该进程的代理环境变量
// 和默认日志记录器；调用方不得在宿主进程中运行它。
func RunEdge(in io.Reader, out io.Writer) error {
	request, err := readEdgeRequest(in)
	if err != nil {
		return err
	}
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if proxyEnv(key) {
			if os.Unsetenv(key) != nil {
				return errors.New("tts: proxy environment cleanup failed")
			}
		}
	}
	log.SetOutput(io.Discard)
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(request.Text)); err != nil {
		return errors.New("tts: invalid synthesis input")
	}
	communicator, err := edge.NewCommunicate(escaped.String(), edge.SetVoice(request.Voice), edge.SetProxy(request.Proxy), edge.SetOutputFormat(edge.OutputFormatMP3), edge.SetReceiveTimeout(edgeReceiveTimeoutSeconds))
	if err != nil {
		return errors.New("tts: invalid synthesis configuration")
	}
	var audio []byte
	attempts := len(edgeRetryDelays)
	// 重试留在专用进程内；宿主的总超时或取消会直接终止进程，包括等待阶段。
	for attempt, delay := range edgeRetryDelays {
		if delay > 0 {
			time.Sleep(delay)
		}
		started := time.Now()
		audio, err = communicator.Stream()
		if err == nil && len(audio) > 0 {
			break
		}
		// 逐次重试的细节写入 stderr，由宿主在失败时记入日志；
		// 只含端点主机、代理协议与错误类别，绝不包含文本或凭据。
		fmt.Fprintf(os.Stderr, "tts: synthesis attempt %d/%d failed (%s, elapsed=%s)\n", attempt+1, attempts, edgeDetail(err, request.Proxy), time.Since(started).Round(time.Millisecond))
		if attempt == attempts-1 {
			if err != nil {
				return safeEdgeError(err, request.Proxy, attempts)
			}
			return fmt.Errorf("tts: synthesis returned empty audio (%s, attempts=%d)", edgeDetail(nil, request.Proxy), attempts)
		}
	}
	if len(audio) > MaxAudioBytes {
		return errors.New("tts: synthesis audio exceeds limit")
	}
	if _, err := out.Write(audio); err != nil {
		return errors.New("tts: writing synthesis audio failed")
	}
	return nil
}
func readEdgeRequest(in io.Reader) (edgeRequest, error) {
	var request edgeRequest
	data, err := io.ReadAll(io.LimitReader(in, maxEdgeRequestBytes+1))
	if err != nil || len(data) > maxEdgeRequestBytes || !utf8.Valid(data) {
		return request, errors.New("tts: invalid synthesis input")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return request, errors.New("tts: invalid synthesis input")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return request, errors.New("tts: invalid synthesis input")
	}
	request.Text, err = validateText(request.Text)
	if err != nil {
		return request, err
	}
	if !validVoice(request.Voice) {
		return request, errors.New("tts: invalid voice")
	}
	request.Proxy, err = normalizeProxy(request.Proxy)
	if err != nil {
		return request, err
	}
	return request, nil
}

var edgeRetryDelays = [...]time.Duration{0, 500 * time.Millisecond, 1500 * time.Millisecond}

// edgeHost 返回合成端点主机；不含 WSS_URL 查询串里的令牌。
func edgeHost() string {
	u, err := url.Parse(edge.WSS_URL)
	if err != nil {
		return "unknown"
	}
	return u.Host
}

// edgeFailureKind 把底层错误归类为不含文本、凭据的简短原因。
func edgeFailureKind(err error) string {
	var timeout net.Error
	var dns *net.DNSError
	var op *net.OpError
	var closeErr *websocket.CloseError
	var verify *tls.CertificateVerificationError
	var errno syscall.Errno
	switch {
	case err == nil:
		return "empty audio"
	case errors.Is(err, websocket.ErrBadHandshake):
		// edge-tts 丢弃了握手响应，拿不到 HTTP 状态码。
		return "bad handshake (HTTP status unavailable)"
	case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout():
		return "timeout"
	case errors.As(err, &dns):
		return "dns lookup failed"
	case errors.As(err, &verify):
		return "tls certificate verification failed"
	case errors.As(err, &closeErr):
		return fmt.Sprintf("websocket closed (code %d)", closeErr.Code)
	case errors.As(err, &op):
		if errors.As(err, &errno) {
			return op.Op + ": " + errno.Error()
		}
		return op.Op + " failed"
	case errors.Is(err, io.EOF), errors.Is(err, io.ErrUnexpectedEOF):
		return "unexpected eof"
	}
	return "protocol error"
}

// edgeDetail 生成可写入 stderr 与日志的诊断细节。
func edgeDetail(err error, proxy string) string {
	return fmt.Sprintf("host=%s, proxy=%s, cause=%s", edgeHost(), proxyLabel(proxy), edgeFailureKind(err))
}

// safeEdgeError 返回带类别前缀的错误：类别是固定文本（宿主按前缀匹配），
// 括号内的细节只会进入日志，不会进入返回给用户的错误。
func safeEdgeError(err error, proxy string, attempts int) error {
	category := "tts: synthesis network or protocol failure"
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		category = "tts: synthesis network timeout"
	} else if errors.Is(err, websocket.ErrBadHandshake) {
		category = "tts: synthesis handshake rejected"
	}
	return fmt.Errorf("%s (%s, attempts=%d)", category, edgeDetail(err, proxy), attempts)
}
