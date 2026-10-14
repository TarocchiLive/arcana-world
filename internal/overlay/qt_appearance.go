package overlay

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
)

// QtAppearance 保存 Qt 专用外观；值类型使完整配置保持可比较。
type QtAppearance struct {
	ShowInTaskbar       bool    `json:"show_in_taskbar"`
	Alignment           string  `json:"alignment"`
	HistoryScale        float64 `json:"history_scale"`
	LatestScale         float64 `json:"latest_scale"`
	LineSpacing         float64 `json:"line_spacing"`
	MessageSpacing      float64 `json:"message_spacing"`
	HistoryAlpha        float64 `json:"history_alpha"`
	MaxMessages         int     `json:"max_messages"`
	MaxLines            int     `json:"max_lines"`
	Gradient            bool    `json:"gradient"`
	AccentStart         string  `json:"accent_start"`
	AccentEnd           string  `json:"accent_end"`
	CornerRadius        float64 `json:"corner_radius"`
	BorderAlpha         float64 `json:"border_alpha"`
	ShadowStrength      float64 `json:"shadow_strength"`
	AnimationEnabled    bool    `json:"animation_enabled"`
	AnimationDurationMS int     `json:"animation_duration_ms"`
	AnimationDistance   float64 `json:"animation_distance"`
}

func DefaultQtAppearance() QtAppearance {
	return QtAppearance{
		Alignment: "left", HistoryScale: .9, LatestScale: 1.05,
		LineSpacing: 3, MessageSpacing: 8, HistoryAlpha: .55, MaxMessages: 6, MaxLines: 4,
		Gradient: true, AccentStart: "#FF4FA3", AccentEnd: "#FF8FCB",
		CornerRadius: 16, BorderAlpha: .18, ShadowStrength: .3,
		AnimationEnabled: true, AnimationDurationMS: 320, AnimationDistance: 10,
	}
}

// UnmarshalJSON 仅为缺省字段补默认值；显式零值和 false 保持不变。
// 自定义解码仍拒绝未知字段，避免绕过协议的严格字段检查。
func (appearance *QtAppearance) UnmarshalJSON(data []byte) error {
	type appearanceJSON QtAppearance
	decoded := appearanceJSON(DefaultQtAppearance())
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&decoded); err != nil {
		return err
	}
	*appearance = QtAppearance(decoded)
	return nil
}

func (appearance QtAppearance) Normalize() (QtAppearance, error) {
	switch appearance.Alignment {
	case "left", "center", "right":
	default:
		return QtAppearance{}, fmt.Errorf("overlay: Elegant overlay alignment must be left, center or right")
	}
	for _, field := range [...]struct {
		name            string
		value, min, max float64
	}{
		{"history_scale", appearance.HistoryScale, .5, 1.5},
		{"latest_scale", appearance.LatestScale, .5, 2},
		{"line_spacing", appearance.LineSpacing, 0, 32},
		{"message_spacing", appearance.MessageSpacing, 0, 64},
		{"history_alpha", appearance.HistoryAlpha, 0, 1},
		{"corner_radius", appearance.CornerRadius, 0, 64},
		{"border_alpha", appearance.BorderAlpha, 0, 1},
		{"shadow_strength", appearance.ShadowStrength, 0, 1},
		{"animation_distance", appearance.AnimationDistance, 0, 128},
	} {
		if math.IsNaN(field.value) || math.IsInf(field.value, 0) || field.value < field.min || field.value > field.max {
			return QtAppearance{}, fmt.Errorf("overlay: Elegant overlay %s must be finite and in %g..%g", field.name, field.min, field.max)
		}
	}
	if appearance.MaxMessages < 1 || appearance.MaxMessages > 6 {
		return QtAppearance{}, fmt.Errorf("overlay: Elegant overlay max_messages must be in 1..6")
	}
	if appearance.MaxLines < 1 || appearance.MaxLines > 12 {
		return QtAppearance{}, fmt.Errorf("overlay: Elegant overlay max_lines must be in 1..12")
	}
	if appearance.AnimationDurationMS < 0 || appearance.AnimationDurationMS > 2000 {
		return QtAppearance{}, fmt.Errorf("overlay: Elegant overlay animation_duration_ms must be in 0..2000")
	}
	for _, color := range [...]struct {
		name  string
		value *string
	}{
		{"accent_start", &appearance.AccentStart}, {"accent_end", &appearance.AccentEnd},
	} {
		if _, err := ParseColor(*color.value); err != nil {
			return QtAppearance{}, fmt.Errorf("overlay: Elegant overlay %s: %w", color.name, err)
		}
		*color.value = strings.ToUpper(*color.value)
	}
	return appearance, nil
}
