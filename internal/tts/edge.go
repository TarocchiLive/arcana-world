package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

var ErrUnsupportedProxy = errors.New("tts: unsupported proxy scheme")

const defaultVoice = "zh-CN-XiaoxiaoNeural"

type EdgeOptions struct {
	Executable string
	Voice      string
	Proxy      string
	Timeout    time.Duration
}
type Edge struct {
	executable, voice, proxy string
	timeout                  time.Duration
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
		options.Timeout = 30 * time.Second
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
	executable, err := resolveHelper("arcana-tts", options.Executable)
	if err != nil {
		return nil, err
	}
	return &Edge{executable: executable, voice: options.Voice, proxy: proxy, timeout: options.Timeout}, nil
}
func (e *Edge) Synthesize(ctx context.Context, text string) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("tts: nil context")
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
		return nil, errors.New("tts: encoding synthesis input")
	}
	if input.Len() > 8<<10 {
		return nil, errors.New("tts: synthesis input exceeds limit")
	}
	audio, err := runHelper(ctx, e.executable, input.Bytes(), MaxAudioBytes, withoutProxyEnv(os.Environ()))
	if err != nil {
		return nil, fmt.Errorf("tts: synthesize: %w", err)
	}
	if len(audio) == 0 {
		return nil, errors.New("tts: synthesis returned empty audio")
	}
	return audio, nil
}
func validVoice(voice string) bool {
	if len(voice) > 96 || !strings.HasSuffix(voice, "Neural") {
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
		request, _ := http.NewRequest(http.MethodGet, "https://speech.platform.bing.com", nil)
		proxy, err := http.ProxyFromEnvironment(request)
		if err != nil {
			return "", errors.New("tts: invalid environment proxy")
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
	if len(value) > 2048 {
		return "", errors.New("tts: invalid proxy")
	}
	proxy, err := url.Parse(value)
	if err != nil {
		return "", errors.New("tts: invalid proxy")
	}
	switch proxy.Scheme {
	case "socks5h":
		proxy.Scheme = "socks5"
	case "http", "socks5":
	default:
		return "", ErrUnsupportedProxy
	}
	if proxy.Opaque != "" || proxy.Hostname() == "" || proxy.Fragment != "" || proxy.RawQuery != "" || proxy.ForceQuery || proxy.Path != "" && proxy.Path != "/" {
		return "", errors.New("tts: invalid proxy")
	}
	host := proxy.Hostname()
	if net.ParseIP(host) == nil {
		for _, label := range strings.Split(host, ".") {
			if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return "", errors.New("tts: invalid proxy hostname")
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
					return "", errors.New("tts: invalid proxy hostname")
				}
			}
		}
	}
	if strings.HasSuffix(proxy.Host, ":") || proxy.Scheme == "socks5" && proxy.Port() == "" {
		return "", errors.New("tts: invalid proxy port")
	}
	if port := proxy.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", errors.New("tts: invalid proxy port")
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
