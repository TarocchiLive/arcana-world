package tts

import (
	"errors"
	"strings"

	"arcana-world/internal/i18n"
)

type Settings struct {
	Voice          string   `json:"voice"`
	Volume         int      `json:"volume"`
	DisabledEvents []string `json:"disabled_events,omitempty"`
}

type Voice struct {
	ID   string
	Name i18n.Key
}

var chineseVoices = []Voice{
	{"zh-CN-XiaoxiaoNeural", i18n.OutputVoiceXiaoxiao},
	{"zh-CN-XiaoyiNeural", i18n.OutputVoiceXiaoyi},
	{"zh-CN-YunxiNeural", i18n.OutputVoiceYunxi},
	{"zh-CN-YunjianNeural", i18n.OutputVoiceYunjian},
	{"zh-CN-YunxiaNeural", i18n.OutputVoiceYunxia},
	{"zh-CN-YunyangNeural", i18n.OutputVoiceYunyang},
	{"zh-HK-HiuGaaiNeural", i18n.OutputVoiceHiuGaai},
	{"zh-HK-WanLungNeural", i18n.OutputVoiceWanLung},
	{"zh-TW-HsiaoChenNeural", i18n.OutputVoiceHsiaoChen},
	{"zh-TW-YunJheNeural", i18n.OutputVoiceYunJhe},
}

func Voices() []Voice { return append([]Voice(nil), chineseVoices...) }

func DefaultSettings() Settings { return Settings{Voice: chineseVoices[0].ID, Volume: 80} }

func (s Settings) Normalize() (Settings, error) {
	if s.Volume < 0 || s.Volume > 100 {
		return Settings{}, errors.New(i18n.T(i18n.OutputVolumeInvalid))
	}
	s.Voice = strings.TrimSpace(s.Voice)
	if s.Voice == "" {
		s.Voice = DefaultSettings().Voice
	}
	for _, voice := range chineseVoices {
		if voice.ID == s.Voice {
			s.DisabledEvents = append([]string(nil), s.DisabledEvents...)
			return s, nil
		}
	}
	return Settings{}, errors.New(i18n.T(i18n.OutputVoiceUnsupported))
}
