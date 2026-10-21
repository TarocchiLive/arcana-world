package tts

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"strings"

	"arcana-world/internal/i18n"
)

type Settings struct {
	Provider       string                       `json:"provider"`
	Providers      map[string]map[string]string `json:"providers"`
	Volume         int                          `json:"volume"`
	DisabledEvents []string                     `json:"disabled_events,omitempty"`
}

// MarshalJSON 将密钥留在凭据后端，普通配置只保存供应商参数。
func (s Settings) MarshalJSON() ([]byte, error) {
	s = s.Clone()
	for _, provider := range providers {
		for _, field := range provider.Fields {
			if field.Secret {
				delete(s.Providers[provider.ID], field.ID)
			}
		}
	}
	type settings Settings
	return json.Marshal(settings(s))
}

// UnmarshalJSON 仅在读取旧配置时迁移 voice，保存时不再写出旧字段。
func (s *Settings) UnmarshalJSON(data []byte) error {
	type settings Settings
	base := *s
	// 供应商参数整体读取，避免默认音色覆盖旧配置的 voice。
	base.Providers = nil
	value := struct {
		settings
		Voice string `json:"voice"`
	}{settings: settings(base.Clone())}
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	result := Settings(value.settings)
	if result.Provider == "" {
		result.Provider = "edge"
	}
	if value.Voice != "" {
		if result.Providers == nil {
			result.Providers = make(map[string]map[string]string)
		}
		if result.Providers["edge"] == nil {
			result.Providers["edge"] = make(map[string]string)
		}
		// 显式的新配置优先于旧字段。
		if _, exists := result.Providers["edge"]["voice"]; !exists {
			result.Providers["edge"]["voice"] = value.Voice
		}
	}
	*s = result
	return nil
}

func DefaultSettings() Settings {
	s := Settings{Provider: "edge", Volume: 80}
	s.Providers = map[string]map[string]string{s.Provider: s.Values()}
	return s
}

// Clone 隔离嵌套配置，避免修改尚未保存的设置影响运行状态。
func (s Settings) Clone() Settings {
	s.Providers = maps.Clone(s.Providers)
	for id, values := range s.Providers {
		s.Providers[id] = maps.Clone(values)
	}
	s.DisabledEvents = append([]string(nil), s.DisabledEvents...)
	return s
}

func (s Settings) providerID() string {
	if id := strings.TrimSpace(s.Provider); id != "" {
		return id
	}
	return "edge"
}

// Values 返回独立的当前提供方参数，补齐默认值并清理首尾空白。
func (s Settings) Values() map[string]string {
	id := s.providerID()
	values := maps.Clone(s.Providers[id])
	if values == nil {
		values = make(map[string]string)
	}
	for key, value := range values {
		values[key] = strings.TrimSpace(value)
	}
	if provider, ok := providerByID(id); ok {
		for _, field := range provider.Fields {
			if values[field.ID] == "" {
				values[field.ID] = field.Default
			}
		}
	}
	return values
}

func (s Settings) WithValue(id, value string) Settings {
	s = s.Clone()
	s.Provider = s.providerID()
	if s.Providers == nil {
		s.Providers = make(map[string]map[string]string)
	}
	values := s.Values()
	values[id] = value
	s.Providers[s.Provider] = values
	return s
}

// Identity 不包含音量和事件开关；JSON 编码保证映射键顺序稳定。
func (s Settings) Identity() string {
	value, _ := json.Marshal(struct {
		Provider string
		Values   map[string]string
	}{s.providerID(), s.Values()})
	return string(value)
}

func (s Settings) Normalize() (Settings, error) {
	if s.Volume < 0 || s.Volume > 100 {
		return Settings{}, errors.New(i18n.T(i18n.OutputVolumeInvalid))
	}
	provider, ok := providerByID(s.providerID())
	if !ok {
		return Settings{}, fmt.Errorf(i18n.T(i18n.OutputProviderUnsupported), s.providerID())
	}
	values := s.Values()
	if err := provider.Validate(values); err != nil {
		return Settings{}, err
	}
	s = s.Clone()
	s.Provider = provider.ID
	if s.Providers == nil {
		s.Providers = make(map[string]map[string]string)
	}
	s.Providers[provider.ID] = values
	return s, nil
}
