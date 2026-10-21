package tts

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"arcana-world/internal/i18n"
	"arcana-world/internal/journal"
	"arcana-world/internal/tts/edgetts"
	"github.com/gorilla/websocket"
)

const (
	defaultVoice            = "zh-CN-XiaoxiaoNeural"
	defaultSynthesisTimeout = 30 * time.Second
	maxVoiceBytes           = 96
	maxProxyBytes           = 2048
)

// edge 通过 Edge 朗读服务在线合成；连接、代理与重试沿用其他在线语音的会话。
type edge struct {
	*cloudSession
	voice  string
	dialer *websocket.Dialer
}

func newEdge(voice, proxy string) (Synthesizer, error) {
	if voice == "" {
		voice = defaultVoice
	}
	// 音色写入 SSML 属性，只接受官方音色名的字符集。
	if !validVoice(voice) {
		return nil, errors.New(i18n.T(i18n.OutputVoiceUnsupported))
	}
	session, err := newCloudSession(i18n.T(i18n.TTSEdgeName), proxy)
	if err != nil {
		return nil, err
	}
	dialer := &websocket.Dialer{Proxy: session.proxy, HandshakeTimeout: defaultSynthesisTimeout, EnableCompression: true}
	logger.Info("edge synthesizer ready", "voice", voice)
	return &edge{cloudSession: session, voice: voice, dialer: dialer}, nil
}

func (e *edge) Synthesize(ctx context.Context, text string) ([]byte, error) {
	text, err := validateText(text)
	if err != nil {
		return nil, err
	}
	return e.synthesize(ctx, func(ctx context.Context) ([]byte, error) {
		audio, err := edgetts.Synthesize(ctx, e.dialer, e.voice, text, MaxAudioBytes)
		if err == nil {
			return audio, nil
		}
		// 日志只记录错误类别，不含文本、令牌或代理凭据。
		logger.Debug("edge synthesis failed", "cause", edgeFailureKind(err))
		var rejected *edgetts.HandshakeError
		switch {
		case errors.Is(err, edgetts.ErrEmptyAudio):
			return nil, retryableError{e.fail(i18n.TTSCloudAudioInvalid)}
		case errors.Is(err, edgetts.ErrAudioTooLarge), errors.Is(err, edgetts.ErrProtocol):
			return nil, e.fail(i18n.TTSCloudAudioInvalid)
		case errors.As(err, &rejected):
			return nil, retryableError{e.fail(i18n.TTSEdgeRejected)}
		}
		return nil, e.failure(ctx, i18n.TTSEdgeUnreachable)
	})
}

// edgeFailureKind 把底层错误归类为不含文本、凭据的简短原因。
func edgeFailureKind(err error) string {
	var timeout net.Error
	var dns *net.DNSError
	var op *net.OpError
	var closeErr *websocket.CloseError
	var verify *tls.CertificateVerificationError
	var rejected *edgetts.HandshakeError
	var errno syscall.Errno
	switch {
	case errors.As(err, &rejected):
		return "handshake rejected (HTTP " + strconv.Itoa(rejected.Status) + ")"
	case errors.Is(err, edgetts.ErrEmptyAudio):
		return "empty audio"
	case errors.Is(err, edgetts.ErrAudioTooLarge):
		return "audio too large"
	case errors.Is(err, edgetts.ErrProtocol):
		return "protocol error"
	case errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout():
		return "timeout"
	case errors.As(err, &dns):
		return "dns lookup failed"
	case errors.As(err, &verify):
		return "tls certificate verification failed"
	case errors.As(err, &closeErr):
		return "websocket closed (code " + strconv.Itoa(closeErr.Code) + ")"
	case errors.As(err, &op):
		if errors.As(err, &errno) {
			return op.Op + ": " + errno.Error()
		}
		return op.Op + " failed"
	}
	return "network error"
}

func validVoice(voice string) bool {
	if len(voice) > maxVoiceBytes || !strings.HasSuffix(voice, "Neural") {
		return false
	}
	for _, c := range voice {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// normalizeProxy 校验用户填写的代理，并把凭据登记为日志敏感信息。
func normalizeProxy(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if len(value) > maxProxyBytes {
		return "", errors.New(i18n.T(i18n.TTSProxyInvalid))
	}
	proxy, err := url.Parse(value)
	if err != nil {
		return "", errors.New(i18n.T(i18n.TTSProxyInvalid))
	}
	// 代理凭据在内存中出现的第一处：登记后日志会自动隐藏。
	if proxy.User != nil {
		journal.AddSecrets(proxy.User.String())
		if password, ok := proxy.User.Password(); ok {
			journal.AddSecrets(password)
		}
	}
	switch proxy.Scheme {
	case "socks5h":
		proxy.Scheme = "socks5"
	case "http", "socks5":
	default:
		return "", errors.New(i18n.T(i18n.TTSProxyUnsupported))
	}
	if proxy.Opaque != "" || proxy.Hostname() == "" || proxy.Fragment != "" || proxy.RawQuery != "" || proxy.ForceQuery || proxy.Path != "" && proxy.Path != "/" {
		return "", errors.New(i18n.T(i18n.TTSProxyInvalid))
	}
	host := proxy.Hostname()
	if net.ParseIP(host) == nil {
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", errors.New(i18n.T(i18n.TTSProxyInvalid))
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
					return "", errors.New(i18n.T(i18n.TTSProxyInvalid))
				}
			}
		}
	}
	if strings.HasSuffix(proxy.Host, ":") || proxy.Scheme == "socks5" && proxy.Port() == "" {
		return "", errors.New(i18n.T(i18n.TTSProxyInvalid))
	}
	if port := proxy.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New(i18n.T(i18n.TTSProxyInvalid))
		}
	}
	return proxy.String(), nil
}
