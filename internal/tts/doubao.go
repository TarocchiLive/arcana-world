package tts

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"arcana-world/internal/i18n"
)

const (
	doubaoDefaultResource = "seed-tts-2.0"
	doubaoDefaultVoice    = "zh_female_vv_uranus_bigtts"
	doubaoEndpoint        = "/api/v3/tts/unidirectional"
	// 除 base64 音频外，预留协议元数据空间，同时限制异常流的总大小。
	doubaoMaxResponseBytes = 32 << 20
)

// doubaoLabel 返回提示中使用的服务名称，随界面语言变化。
func doubaoLabel() string { return i18n.T(i18n.TTSDoubaoName) }

type doubaoSynthesizer struct {
	*cloudSession
	endpoint string
	apiKey   string
	resource string
	voice    string
}

func validateDoubao(values map[string]string) error {
	if len(values["base_url"]) > 4096 {
		return fmt.Errorf(i18n.T(i18n.TTSBaseURLInvalid), doubaoLabel())
	}
	if value := values["base_url"]; value != "" {
		if _, err := doubaoURL(value); err != nil {
			return err
		}
	}
	for _, name := range []struct {
		id    string
		label i18n.Key
	}{{"api_key", i18n.TTSAPIKey}, {"resource_id", i18n.TTSResourceID}, {"voice", i18n.OutputVoice}} {
		value := values[name.id]
		if len(value) > 4096 || strings.IndexFunc(value, func(r rune) bool { return r <= 0x20 || r >= 0x7f }) >= 0 {
			return fmt.Errorf(i18n.T(i18n.TTSFieldInvalid), doubaoLabel(), i18n.T(name.label))
		}
	}
	return nil
}

func doubaoURL(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" {
		return "", fmt.Errorf(i18n.T(i18n.TTSBaseURLInvalid), doubaoLabel())
	}
	path := strings.TrimRight(u.Path, "/")
	switch path {
	case "", "/api/v3", "/api/v3/tts":
		u.Path = doubaoEndpoint
		u.RawPath = ""
	default:
		if strings.HasSuffix(path, "/sse") {
			return "", errors.New(i18n.T(i18n.TTSDoubaoSSE))
		}
		// 自定义网关路径按完整接口地址使用，不猜测或重写其前缀。
		u.Path = path
		u.RawPath = ""
	}
	return u.String(), nil
}

func newDoubao(values map[string]string, proxy string) (Synthesizer, error) {
	if err := validateDoubao(values); err != nil {
		return nil, err
	}
	if values["base_url"] == "" || values["api_key"] == "" {
		return nil, fmt.Errorf(i18n.T(i18n.TTSCredentialsRequired), doubaoLabel())
	}
	endpoint, err := doubaoURL(values["base_url"])
	if err != nil {
		return nil, err
	}
	session, err := newCloudSession(doubaoLabel(), proxy)
	if err != nil {
		return nil, err
	}
	resource, voice := values["resource_id"], values["voice"]
	if resource == "" {
		resource = doubaoDefaultResource
	}
	if voice == "" {
		voice = doubaoDefaultVoice
	}
	return &doubaoSynthesizer{cloudSession: session, endpoint: endpoint, apiKey: values["api_key"], resource: resource, voice: voice}, nil
}

func (s *doubaoSynthesizer) Synthesize(ctx context.Context, text string) ([]byte, error) {
	text, err := validateText(text)
	if err != nil {
		return nil, err
	}
	ctx, done, err := s.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	payload := struct {
		User struct {
			UID string `json:"uid"`
		} `json:"user"`
		Params struct {
			Text    string `json:"text"`
			Speaker string `json:"speaker"`
			Audio   struct {
				Format     string `json:"format"`
				SampleRate int    `json:"sample_rate"`
			} `json:"audio_params"`
		} `json:"req_params"`
	}{}
	payload.User.UID = "arcana"
	payload.Params.Text = text
	payload.Params.Speaker = s.voice
	// 流式 WAV 会重复携带文件头，因此固定请求可直接拼接的 MP3。
	payload.Params.Audio.Format = "mp3"
	payload.Params.Audio.SampleRate = 24000
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("encoding Doubao request failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("Doubao request could not be created")
	}
	// 付费合成不自动重放请求。
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", s.apiKey)
	req.Header.Set("X-Api-Resource-Id", s.resource)
	response, err := s.client.Do(req)
	if err != nil {
		// 不透传 URL、服务端消息或底层网络错误，以免泄漏用户配置及凭据。
		return nil, s.failure(ctx, i18n.TTSCloudUnreachable)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, s.statusError(response.StatusCode)
	}
	audio, err := readDoubaoAudio(response.Body)
	if err != nil {
		if ctx.Err() != nil || s.lifetime.Err() != nil {
			return nil, s.failure(ctx, i18n.TTSCloudInterrupted)
		}
		return nil, err
	}
	if ctx.Err() != nil || s.lifetime.Err() != nil {
		return nil, s.failure(ctx, i18n.TTSCloudInterrupted)
	}
	return audio, nil
}

func readDoubaoAudio(reader io.Reader) ([]byte, error) {
	limited := &io.LimitedReader{R: reader, N: doubaoMaxResponseBytes + 1}
	decoder := json.NewDecoder(limited)
	var audio []byte
	finished := false
	for {
		var packet struct {
			Code *int   `json:"code"`
			Data string `json:"data"`
		}
		err := decoder.Decode(&packet)
		if limited.N == 0 {
			return nil, fmt.Errorf(i18n.T(i18n.TTSCloudBadResponse), doubaoLabel())
		}
		if err == io.EOF {
			if !finished {
				return nil, fmt.Errorf(i18n.T(i18n.TTSCloudInterrupted), doubaoLabel())
			}
			if len(audio) == 0 {
				return nil, fmt.Errorf(i18n.T(i18n.TTSCloudAudioInvalid), doubaoLabel())
			}
			return audio, nil
		}
		if err != nil {
			return nil, fmt.Errorf(i18n.T(i18n.TTSCloudBadResponse), doubaoLabel())
		}
		if finished || packet.Code == nil {
			return nil, fmt.Errorf(i18n.T(i18n.TTSCloudBadResponse), doubaoLabel())
		}
		switch *packet.Code {
		case 0:
		case 20000000:
			finished = true
		default:
			return nil, fmt.Errorf(i18n.T(i18n.TTSCloudRejected), doubaoLabel())
		}
		if packet.Data == "" {
			continue
		}
		// 先检查上限，再为本块扩容；解码直接追加到最终音频，避免额外音频副本。
		remaining := MaxAudioBytes - len(audio)
		if len(packet.Data) > base64.StdEncoding.EncodedLen(remaining) {
			return nil, fmt.Errorf(i18n.T(i18n.TTSCloudAudioInvalid), doubaoLabel())
		}
		previous := len(audio)
		audio, err = base64.StdEncoding.Strict().AppendDecode(audio, []byte(packet.Data))
		if err != nil || len(audio) == previous {
			return nil, fmt.Errorf(i18n.T(i18n.TTSCloudAudioInvalid), doubaoLabel())
		}
		if len(audio) > MaxAudioBytes {
			return nil, fmt.Errorf(i18n.T(i18n.TTSCloudAudioInvalid), doubaoLabel())
		}
	}
}
