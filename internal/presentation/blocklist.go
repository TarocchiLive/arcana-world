package presentation

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"arcana-world/internal/danmaku"
)

const (
	// MaxBlockWords 限制每个列表的词数，使逐条匹配和网页请求保持有界。
	MaxBlockWords = 200
	// MaxBlockWordRunes 限制单个屏蔽词的字符数。
	MaxBlockWordRunes = 50
)

var (
	ErrTooManyBlockWords = errors.New("presentation: too many block words")
	ErrBlockWordTooLong  = errors.New("presentation: block word too long")
)

// Blocklist 保存两类屏蔽词：全局词同时隐藏弹幕和跳过播报，播报词只跳过播报。
// 只影响显示与播报，弹幕历史仍完整保存，移除屏蔽词后重新显示。
type Blocklist struct {
	Global []string `json:"global,omitempty"`
	TTS    []string `json:"tts,omitempty"`
}

func (b Blocklist) Clone() Blocklist {
	return Blocklist{Global: slices.Clone(b.Global), TTS: slices.Clone(b.TTS)}
}

func (b Blocklist) Equal(other Blocklist) bool {
	return slices.Equal(b.Global, other.Global) && slices.Equal(b.TTS, other.TTS)
}

// List 按标识返回对应列表的指针；未知标识返回 nil。
func (b *Blocklist) List(id string) *[]string {
	switch id {
	case "global":
		return &b.Global
	case "tts":
		return &b.TTS
	}
	return nil
}

func (b Blocklist) Normalize() (Blocklist, error) {
	var err error
	for _, list := range [...]*[]string{&b.Global, &b.TTS} {
		if *list, err = NormalizeBlockWords(*list); err != nil {
			return Blocklist{}, err
		}
	}
	return b, nil
}

// NormalizeBlockWords 先按分隔符拆分每一项，再去除首尾空白和控制符、丢弃空词，
// 并按匹配规则去重，保留首次出现的写法。终端和网页提交的原始输入都在这里统一拆分。
func NormalizeBlockWords(words []string) ([]string, error) {
	var out []string
	seen := make(map[string]struct{}, len(words))
	for _, item := range words {
		for _, word := range splitBlockWords(item) {
			word = strings.TrimSpace(strings.Map(func(r rune) rune {
				if unicode.IsControl(r) {
					return -1
				}
				return r
			}, strings.ToValidUTF8(word, "")))
			if word == "" {
				continue
			}
			if utf8.RuneCountInString(word) > MaxBlockWordRunes {
				return nil, ErrBlockWordTooLong
			}
			key := foldBlockText(word)
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, word)
		}
	}
	if len(out) > MaxBlockWords {
		return nil, ErrTooManyBlockWords
	}
	return out, nil
}

// splitBlockWords 按换行、逗号、顿号和分号拆分一次输入的多个屏蔽词；空格属于词的一部分。
func splitBlockWords(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool {
		switch r {
		case '\n', '\r', ',', '，', '、', ';', '；':
			return true
		}
		return false
	})
}

// BlockMatcher 保存折叠后的屏蔽词，匹配不区分大小写和全角／半角。
type BlockMatcher []string

func NewBlockMatcher(lists ...[]string) BlockMatcher {
	var matcher BlockMatcher
	for _, list := range lists {
		for _, word := range list {
			if word = foldBlockText(word); word != "" && !slices.Contains(matcher, word) {
				matcher = append(matcher, word)
			}
		}
	}
	return matcher
}

// DisplayMatcher 只含全局词，作用于弹幕列表、历史、网页、浮层和实时广播。
func (b Blocklist) DisplayMatcher() BlockMatcher { return NewBlockMatcher(b.Global) }

// TTSMatcher 合并全局词和播报词：被隐藏的弹幕同样不播报。
func (b Blocklist) TTSMatcher() BlockMatcher { return NewBlockMatcher(b.Global, b.TTS) }

// Blocks 检查事件中用户可见的文本：昵称、弹幕内容、礼物名、标题和详情字段。
// 详情事件的 Text 是协议命令名，不参与匹配。
func (m BlockMatcher) Blocks(e danmaku.Event) bool {
	if len(m) == 0 {
		return false
	}
	if m.contains(e.User) || m.contains(e.Gift) || m.contains(e.Title) || e.Kind != "detail" && m.contains(e.Text) {
		return true
	}
	for _, field := range e.Fields {
		if m.contains(field.Value) {
			return true
		}
	}
	return false
}

func (m BlockMatcher) contains(value string) bool {
	if value == "" {
		return false
	}
	// strings.Map 在没有字符变化时直接返回原字符串，常见中文弹幕无需额外分配。
	value = foldBlockText(value)
	for _, word := range m {
		if strings.Contains(value, word) {
			return true
		}
	}
	return false
}

func foldBlockText(value string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '！' && r <= '～':
			r -= '！' - '!'
		case r == '\u3000':
			r = ' '
		}
		return unicode.ToLower(r)
	}, value)
}
