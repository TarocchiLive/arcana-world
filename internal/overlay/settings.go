package overlay

import (
	"encoding/json"
	"errors"
)

// Settings 保存展示偏好，不包含动态渲染文本。
type Settings struct {
	Enabled         bool     `json:"enabled"`
	Content         string   `json:"content"`
	Position        Position `json:"position"`
	Width           float64  `json:"width"`
	Height          float64  `json:"height"`
	Padding         Insets   `json:"padding"`
	Font            Font     `json:"font"`
	TextAlpha       float64  `json:"text_alpha"`
	BackgroundAlpha float64  `json:"background_alpha"`
	DisplayID       uint32   `json:"display_id"`
	Output          string   `json:"output"`
}

func DefaultSettings() Settings {
	cfg := DefaultConfig()
	return Settings{
		Content: "danmaku", Position: cfg.Position,
		Width: cfg.Width, Height: cfg.Height, Padding: cfg.Padding, Font: cfg.Font,
		TextAlpha: cfg.TextAlpha, BackgroundAlpha: cfg.BackgroundAlpha,
		DisplayID: cfg.DisplayID, Output: cfg.Output,
	}
}

// UnmarshalJSON 在完整默认值上解码，保留显式零值及缺省的嵌套字段。
func (s *Settings) UnmarshalJSON(data []byte) error {
	type settingsJSON Settings
	decoded := settingsJSON(DefaultSettings())
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	normalized, err := Settings(decoded).Normalize()
	if err != nil {
		return err
	}
	*s = normalized
	return nil
}

func (s Settings) Normalize() (Settings, error) {
	switch s.Content {
	case "status", "danmaku", "combined":
	default:
		return Settings{}, errors.New("overlay: invalid content mode")
	}
	if _, err := s.Config("").Normalize(); err != nil {
		return Settings{}, err
	}
	return s, nil
}

func (s Settings) Config(text string) Config {
	return Config{
		Text: text, Position: s.Position,
		Width: s.Width, Height: s.Height, Padding: s.Padding, Font: s.Font,
		TextAlpha: s.TextAlpha, BackgroundAlpha: s.BackgroundAlpha,
		DisplayID: s.DisplayID, Output: s.Output,
	}
}
