package tts

import (
	"errors"
	"strings"
)

type Settings struct {
	Voice          string   `json:"voice"`
	DisabledEvents []string `json:"disabled_events,omitempty"`
}

type Voice struct {
	ID, Name string
}

var chineseVoices = []Voice{
	{"zh-CN-XiaoxiaoNeural", "晓晓（普通话女声）"},
	{"zh-CN-XiaoyiNeural", "晓伊（普通话女声）"},
	{"zh-CN-YunxiNeural", "云希（普通话男声）"},
	{"zh-CN-YunjianNeural", "云健（普通话男声）"},
	{"zh-CN-YunxiaNeural", "云夏（普通话男声）"},
	{"zh-CN-YunyangNeural", "云扬（普通话男声）"},
	{"zh-HK-HiuGaaiNeural", "晓佳（粤语女声）"},
	{"zh-HK-WanLungNeural", "云龙（粤语男声）"},
	{"zh-TW-HsiaoChenNeural", "晓臻（台湾普通话女声）"},
	{"zh-TW-YunJheNeural", "云哲（台湾普通话男声）"},
}

func Voices() []Voice { return append([]Voice(nil), chineseVoices...) }

func DefaultSettings() Settings { return Settings{Voice: chineseVoices[0].ID} }

func (s Settings) Normalize() (Settings, error) {
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
	return Settings{}, errors.New("不支持的中文语音音色")
}
