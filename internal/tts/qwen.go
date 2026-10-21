package tts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode/utf8"

	"arcana-world/internal/i18n"
)

const (
	qwenDefaultModel    = "qwen3-tts-flash"
	qwenDefaultVoice    = "Cherry"
	qwenDefaultLanguage = "Auto"
	qwenServicePath     = "/services/aigc/multimodal-generation/generation"
	qwenResponseLimit   = 64 << 10
)

// qwenLabel 返回提示中使用的服务名称，随界面语言变化。
func qwenLabel() string { return i18n.T(i18n.TTSQwenName) }

type qwen struct {
	*cloudSession
	endpoint, key, model, voice, language string
}

// qwenEndpoint 支持站点根、/api/v1 前缀、自定义 API 前缀及完整接口地址。
func qwenEndpoint(value string) (string, error) {
	u, err := url.Parse(value)
	if err != nil || len(value) > 4096 || u == nil || u.Hostname() == "" ||
		(u.Scheme != "https" && u.Scheme != "http") || u.User != nil ||
		u.Opaque != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return "", fmt.Errorf(i18n.T(i18n.TTSBaseURLInvalid), qwenLabel())
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		path = "/api/v1"
	}
	if !strings.HasSuffix(path, qwenServicePath) {
		path += qwenServicePath
	}
	u.Path, u.RawPath = path, ""
	return u.String(), nil
}

func validateQwen(values map[string]string) error {
	if value := values["base_url"]; value != "" {
		if _, err := qwenEndpoint(value); err != nil {
			return err
		}
	}
	if key := values["api_key"]; len(key) > 4096 || strings.IndexFunc(key, func(r rune) bool { return r <= 0x20 || r >= 0x7f }) >= 0 {
		return fmt.Errorf(i18n.T(i18n.TTSAPIKeyInvalid), qwenLabel())
	}
	for _, field := range []struct {
		id    string
		label i18n.Key
	}{{"model", i18n.TTSCloudModel}, {"voice", i18n.OutputVoice}, {"language", i18n.TTSLanguage}} {
		value := values[field.id]
		if len(value) > 256 || !utf8.ValidString(value) || strings.TrimSpace(value) != value || strings.IndexFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) >= 0 {
			return fmt.Errorf(i18n.T(i18n.TTSFieldInvalid), qwenLabel(), i18n.T(field.label))
		}
	}
	return nil
}

func newQwen(values map[string]string, proxy string) (Synthesizer, error) {
	if err := validateQwen(values); err != nil {
		return nil, err
	}
	if values["base_url"] == "" || values["api_key"] == "" {
		return nil, fmt.Errorf(i18n.T(i18n.TTSCredentialsRequired), qwenLabel())
	}
	endpoint, err := qwenEndpoint(values["base_url"])
	if err != nil {
		return nil, err
	}
	session, err := newCloudSession(qwenLabel(), proxy)
	if err != nil {
		return nil, err
	}
	model, voice, language := values["model"], values["voice"], values["language"]
	if model == "" {
		model = qwenDefaultModel
	}
	if voice == "" {
		voice = qwenDefaultVoice
	}
	if language == "" {
		language = qwenDefaultLanguage
	}
	return &qwen{cloudSession: session, endpoint: endpoint, key: values["api_key"], model: model, voice: voice, language: language}, nil
}

func (q *qwen) Synthesize(ctx context.Context, text string) ([]byte, error) {
	text, err := validateText(text)
	if err != nil {
		return nil, err
	}
	ctx, done, err := q.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	payload := struct {
		Model string `json:"model"`
		Input struct {
			Text     string `json:"text"`
			Voice    string `json:"voice"`
			Language string `json:"language_type"`
		} `json:"input"`
	}{Model: q.model}
	payload.Input.Text, payload.Input.Voice, payload.Input.Language = text, q.voice, q.language
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, errors.New("encoding Qwen request failed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, q.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, errors.New("Qwen request could not be created")
	}
	// 禁止传输层重放付费请求；不设置幂等键，也不执行应用层重试。
	req.GetBody = nil
	req.Header.Set("Authorization", "Bearer "+q.key)
	req.Header.Set("Content-Type", "application/json")
	response, err := q.client.Do(req)
	if err != nil {
		return nil, q.failure(ctx, i18n.TTSCloudUnreachable)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, q.statusError(response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, qwenResponseLimit+1))
	if err != nil {
		return nil, q.failure(ctx, i18n.TTSCloudInterrupted)
	}
	if len(data) > qwenResponseLimit {
		return nil, q.fail(i18n.TTSCloudBadResponse)
	}
	var result struct {
		StatusCode int    `json:"status_code"`
		Code       string `json:"code"`
		Output     struct {
			FinishReason string `json:"finish_reason"`
			Audio        struct {
				URL string `json:"url"`
			} `json:"audio"`
		} `json:"output"`
	}
	if json.Unmarshal(data, &result) != nil {
		return nil, q.fail(i18n.TTSCloudBadResponse)
	}
	if result.Code != "" || (result.StatusCode != 0 && result.StatusCode != http.StatusOK) {
		return nil, q.fail(i18n.TTSCloudRejected)
	}
	if result.Output.FinishReason != "" && result.Output.FinishReason != "stop" {
		return nil, q.fail(i18n.TTSCloudAudioInvalid)
	}
	return q.download(ctx, result.Output.Audio.URL)
}

func (q *qwen) download(ctx context.Context, address string) ([]byte, error) {
	u, err := url.Parse(address)
	if err != nil || len(address) > 16384 || u == nil || u.Hostname() == "" ||
		(u.Scheme != "https" && u.Scheme != "http") || u.User != nil || u.Opaque != "" || u.Fragment != "" {
		return nil, q.fail(i18n.TTSCloudBadResponse)
	}
	// 官方返回的 OSS 链接可能使用 HTTP；使用独立请求且绝不附带 API key。
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, errors.New("Qwen audio request could not be created")
	}
	response, err := q.client.Do(req)
	if err != nil {
		return nil, q.failure(ctx, i18n.TTSCloudInterrupted)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, q.statusError(response.StatusCode)
	}
	if response.ContentLength > MaxAudioBytes {
		return nil, q.fail(i18n.TTSCloudAudioInvalid)
	}
	audio, err := io.ReadAll(io.LimitReader(response.Body, MaxAudioBytes+1))
	if err != nil {
		return nil, q.failure(ctx, i18n.TTSCloudInterrupted)
	}
	if len(audio) > MaxAudioBytes {
		return nil, q.fail(i18n.TTSCloudAudioInvalid)
	}
	if len(audio) == 0 {
		return nil, q.fail(i18n.TTSCloudAudioInvalid)
	}
	// 只确认是 WAV 容器；流式头部的长度可能为 0 或 0xFFFFFFFF，采样布局由音频播放器校验。
	if len(audio) < 44 || string(audio[:4]) != "RIFF" || string(audio[8:12]) != "WAVE" {
		return nil, q.fail(i18n.TTSCloudAudioInvalid)
	}
	if err := ctx.Err(); err != nil {
		return nil, q.failure(ctx, i18n.TTSCloudInterrupted)
	}
	return audio, nil
}
