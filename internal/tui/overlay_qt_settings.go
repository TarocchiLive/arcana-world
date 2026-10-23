package tui

import (
	"fmt"
	"math"
	"runtime"
	"strconv"
	"strings"

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
)

var overlayQtGroups = []struct {
	key    string
	label  i18n.Key
	fields []overlaySettingField
}{
	{"qt-text", i18n.TUIOverlayQtText, []overlaySettingField{
		{key: "qt-align", label: i18n.TUIOverlayQtAlignment, read: func(s overlay.Settings) string { return overlayQtAlignmentLabel(s.Qt.Alignment) }},
		overlayQtNumberField("qt-history-scale", i18n.TUIOverlayQtHistoryScale, 0.5, 1.5, false, func(s overlay.Settings) float64 { return s.Qt.HistoryScale }, func(s *overlay.Settings, n float64) { s.Qt.HistoryScale = n }),
		overlayQtNumberField("qt-latest-scale", i18n.TUIOverlayQtLatestScale, 0.5, 2, false, func(s overlay.Settings) float64 { return s.Qt.LatestScale }, func(s *overlay.Settings, n float64) { s.Qt.LatestScale = n }),
		overlayQtNumberField("qt-line-spacing", i18n.TUIOverlayQtLineSpacing, 0, 32, false, func(s overlay.Settings) float64 { return s.Qt.LineSpacing }, func(s *overlay.Settings, n float64) { s.Qt.LineSpacing = n }),
		overlayQtNumberField("qt-message-spacing", i18n.TUIOverlayQtMessageSpacing, 0, 64, false, func(s overlay.Settings) float64 { return s.Qt.MessageSpacing }, func(s *overlay.Settings, n float64) { s.Qt.MessageSpacing = n }),
		overlayQtNumberField("qt-history-alpha", i18n.TUIOverlayQtHistoryAlpha, 0, 1, false, func(s overlay.Settings) float64 { return s.Qt.HistoryAlpha }, func(s *overlay.Settings, n float64) { s.Qt.HistoryAlpha = n }),
		overlayQtNumberField("qt-max-messages", i18n.TUIOverlayQtMaxMessages, 1, 6, true, func(s overlay.Settings) float64 { return float64(s.Qt.MaxMessages) }, func(s *overlay.Settings, n float64) { s.Qt.MaxMessages = int(n) }),
		overlayQtNumberField("qt-max-lines", i18n.TUIOverlayQtMaxLines, 1, 12, true, func(s overlay.Settings) float64 { return float64(s.Qt.MaxLines) }, func(s *overlay.Settings, n float64) { s.Qt.MaxLines = int(n) }),
	}},
	{"qt-panel", i18n.TUIOverlayQtPanel, []overlaySettingField{
		{key: "qt-gradient", label: i18n.TUIOverlayQtGradient, read: func(s overlay.Settings) string { return toggleLabel("", s.Qt.Gradient) }},
		overlayQtColorField("qt-accent-start", i18n.TUIOverlayQtAccentStart, func(q *overlay.QtAppearance) *string { return &q.AccentStart }),
		overlayQtColorField("qt-accent-end", i18n.TUIOverlayQtAccentEnd, func(q *overlay.QtAppearance) *string { return &q.AccentEnd }),
		overlayQtNumberField("qt-radius", i18n.TUIOverlayQtCornerRadius, 0, 64, false, func(s overlay.Settings) float64 { return s.Qt.CornerRadius }, func(s *overlay.Settings, n float64) { s.Qt.CornerRadius = n }),
		overlayQtNumberField("qt-border-alpha", i18n.TUIOverlayQtBorderAlpha, 0, 1, false, func(s overlay.Settings) float64 { return s.Qt.BorderAlpha }, func(s *overlay.Settings, n float64) { s.Qt.BorderAlpha = n }),
		overlayQtNumberField("qt-shadow", i18n.TUIOverlayQtShadowStrength, 0, 1, false, func(s overlay.Settings) float64 { return s.Qt.ShadowStrength }, func(s *overlay.Settings, n float64) { s.Qt.ShadowStrength = n }),
	}},
	{"qt-motion", i18n.TUIOverlayQtMotion, []overlaySettingField{
		{key: "qt-animation", label: i18n.TUIOverlayQtAnimationEnabled, read: func(s overlay.Settings) string { return toggleLabel("", s.Qt.AnimationEnabled) }},
		overlayQtNumberField("qt-animation-ms", i18n.TUIOverlayQtAnimationDuration, 0, 2000, true, func(s overlay.Settings) float64 { return float64(s.Qt.AnimationDurationMS) }, func(s *overlay.Settings, n float64) { s.Qt.AnimationDurationMS = int(n) }),
		overlayQtNumberField("qt-animation-distance", i18n.TUIOverlayQtAnimationDistance, 0, 128, false, func(s overlay.Settings) float64 { return s.Qt.AnimationDistance }, func(s *overlay.Settings, n float64) { s.Qt.AnimationDistance = n }),
		{key: "qt-idle-hide", label: i18n.TUIOverlayQtIdleHide, read: func(s overlay.Settings) string { return toggleLabel("", s.Qt.IdleHideEnabled) }},
		overlayQtNumberField("qt-idle-seconds", i18n.TUIOverlayQtIdleSeconds, 1, 600, true, func(s overlay.Settings) float64 { return float64(s.Qt.IdleHideSeconds) }, func(s *overlay.Settings, n float64) { s.Qt.IdleHideSeconds = int(n) }),
	}},
}

func overlayQtAlignmentLabel(value string) string {
	switch value {
	case "center":
		return i18n.T(i18n.TUIOverlayQtAlignCenter)
	case "right":
		return i18n.T(i18n.TUIOverlayQtAlignRight)
	default:
		return i18n.T(i18n.TUIOverlayQtAlignLeft)
	}
}

func overlayQtNumberField(key string, label i18n.Key, low, high float64, integer bool, get func(overlay.Settings) float64, set func(*overlay.Settings, float64)) overlaySettingField {
	field := overlayNumberField(key, label, get, set)
	field.assign = func(s *overlay.Settings, value string) error {
		n, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < low || n > high || (integer && math.Trunc(n) != n) {
			return fmt.Errorf("%s: %s", i18n.T(i18n.TUIOverlayInvalidNumber), i18n.T(label))
		}
		set(s, n)
		return nil
	}
	return field
}

func overlayQtColorField(key string, label i18n.Key, color func(*overlay.QtAppearance) *string) overlaySettingField {
	return overlaySettingField{
		key: key, label: label, qtColor: color,
		read: func(s overlay.Settings) string { return *color(&s.Qt) },
		assign: func(s *overlay.Settings, value string) error {
			if _, err := overlay.ParseColor(value); err != nil {
				return fmt.Errorf("%s", i18n.T(i18n.TUIOverlayInvalidColor))
			}
			*color(&s.Qt) = strings.ToUpper(value)
			return nil
		},
	}
}

// 与共享字段共用编辑、预览和步进流程，专用分组仅在精美浮层下显示。
func init() {
	for index, group := range overlayQtGroups {
		for _, field := range group.fields {
			field.group = overlaySettingsQtText + index
			if field.step != 0 && (strings.Contains(field.key, "scale") || strings.Contains(field.key, "alpha") || field.key == "qt-shadow") {
				field.step = .01
			}
			overlayFields = append(overlayFields, field)
		}
	}
}

func overlayBackendLabel(backend overlay.Backend) string {
	if backend == overlay.BackendQt {
		return i18n.T(i18n.TUIOverlayElegant)
	}
	return i18n.T(i18n.TUIOverlayNative)
}

func overlayTaskbarSupported() bool {
	return runtime.GOOS == "darwin" || runtime.GOOS == "windows"
}

func overlayTaskbarLabel() string {
	if runtime.GOOS == "darwin" {
		return i18n.T(i18n.TUIOverlayShowInDock)
	}
	return i18n.T(i18n.TUIOverlayShowInTaskbar)
}
