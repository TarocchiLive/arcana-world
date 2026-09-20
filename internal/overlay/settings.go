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
	Outline         bool     `json:"outline"`
	TextAlpha       float64  `json:"text_alpha"`
	BackgroundAlpha float64  `json:"background_alpha"`
	Colors          Colors   `json:"colors"`
	DisplayID       uint32   `json:"display_id"`
	Output          string   `json:"output"`
	Displays        []string `json:"displays"`
}

func DefaultSettings() Settings {
	cfg := DefaultConfig()
	return Settings{
		Content: "danmaku", Position: cfg.Position,
		Width: cfg.Width, Height: cfg.Height, Padding: cfg.Padding, Font: cfg.Font,
		Outline:   cfg.Outline,
		TextAlpha: cfg.TextAlpha, BackgroundAlpha: cfg.BackgroundAlpha,
		Colors:    cfg.Colors,
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
	cfg, err := s.Config("").Normalize()
	if err != nil {
		return Settings{}, err
	}
	s.Colors = cfg.Colors
	return s, nil
}

func (s Settings) Config(text string) Config {
	displays := ""
	if s.Displays != nil {
		encoded, _ := json.Marshal(s.Displays)
		displays = string(encoded)
	}
	return Config{
		Text: text, Position: s.Position,
		Width: s.Width, Height: s.Height, Padding: s.Padding, Font: s.Font,
		Outline:   s.Outline,
		TextAlpha: s.TextAlpha, BackgroundAlpha: s.BackgroundAlpha,
		Colors:    s.Colors,
		DisplayID: s.DisplayID, Output: s.Output,
		Displays: displays,
	}
}
