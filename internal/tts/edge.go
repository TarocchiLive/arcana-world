package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"arcana-world/internal/i18n"
	"arcana-world/internal/journal"
)

const (
	defaultVoice            = "zh-CN-XiaoxiaoNeural"
	defaultSynthesisTimeout = 30 * time.Second
	maxEdgeRequestBytes     = 8 << 10
	maxVoiceBytes           = 96
	maxProxyBytes           = 2048
	edgeWorkerReady         = "edge-tts-v1"
)

type EdgeOptions struct {
	Executable string
	Voice      string
	Proxy      string
	Timeout    time.Duration
}
type Edge struct {
	executable, voice, proxy string
	timeout                  time.Duration
	client                   *residentClient
}
type edgeRequest struct {
	Text  string `json:"text"`
	Voice string `json:"voice"`
	Proxy string `json:"proxy"`
}

func NewEdge(options EdgeOptions) (*Edge, error) {
	if options.Timeout < 0 {
		return nil, errors.New("tts: negative synthesis timeout")
	}
	if options.Timeout == 0 {
		options.Timeout = defaultSynthesisTimeout
	}
	if options.Voice == "" {
		options.Voice = defaultVoice
	}
	if !validVoice(options.Voice) {
		return nil, errors.New("tts: invalid voice")
	}
	proxy, err := effectiveProxy(options.Proxy)
	if err != nil {
		return nil, err
	}
	executable, err := resolveHelper("arcana-world-tts", options.Executable)
	if err != nil {
		return nil, err
	}
	logger.Info("edge synthesizer ready", "executable", executable, "voice", options.Voice, "proxy", proxyLabel(proxy), "timeout", options.Timeout)
	return &Edge{executable: executable, voice: options.Voice, proxy: proxy, timeout: options.Timeout, client: newResidentClient()}, nil
}

func (e *Edge) Close() error { return e.client.Close() }

func (e *Edge) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("nil context")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	text, err := validateText(text)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, e.timeout)
	defer cancel()
	var input bytes.Buffer
	encoder := json.NewEncoder(&input)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(edgeRequest{Text: text, Voice: e.voice, Proxy: e.proxy}); err != nil {
		return nil, errors.New("encoding synthesis input")
	}
	if input.Len() > maxEdgeRequestBytes {
		return nil, errors.New("synthesis input exceeds limit")
	}
	// 错误由 Manager 统一添加 "tts: synthesize:" 前缀，此处原样返回。
	audio, err := e.client.request(ctx, func(context.Context) (*exec.Cmd, error) {
		cmd := exec.Command(e.executable, "edge-worker")
		cmd.Env = withoutProxyEnv(os.Environ())
		return cmd, nil
	}, edgeWorkerReady, input.Bytes(), MaxAudioBytes, nil)
	if err != nil {
		return nil, err
	}
	if len(audio) == 0 {
		return nil, errors.New("synthesis returned empty audio")
	}
	return audio, nil
}

// proxyLabel 返回可安全写入日志的代理描述：只含协议与主机，不含用户信息。
func proxyLabel(proxy string) string {
	if proxy == "" {
		return "direct"
	}
	u, err := url.Parse(proxy)
	if err != nil {
		return "invalid"
	}
	return u.Scheme + "://" + u.Host
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
func effectiveProxy(value string) (string, error) {
	if value == "direct" {
		return "", nil
	}
	if value == "" {
		request, _ := http.NewRequest(http.MethodGet, "https://api.msedgeservices.com", nil)
		proxy, err := http.ProxyFromEnvironment(request)
		if err != nil {
			// 错误文本可能包含环境变量里的代理 URL（含凭据），因此不记录 err。
			logger.Warn("environment proxy invalid")
			return "", errors.New(i18n.T(i18n.TTSProxyInvalid))
		}
		if proxy == nil {
			return "", nil
		}
		value = proxy.String()
	}
	return normalizeProxy(value)
}
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
func proxyEnv(key string) bool {
	switch strings.ToUpper(key) {
	case "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY":
		return true
	}
	return false
}
func withoutProxyEnv(environ []string) []string {
	clean := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, _, _ := strings.Cut(entry, "=")
		if !proxyEnv(key) {
			clean = append(clean, entry)
		}
	}
	return clean
}
