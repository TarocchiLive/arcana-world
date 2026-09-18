package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	tea "github.com/charmbracelet/bubbletea"
)

type overlaySettingField struct {
	key    string
	label  i18n.Key
	read   func(overlay.Settings) string
	assign func(*overlay.Settings, string) error
}

// 普通字段自行解析和读写；assign 为空的交互由 chooseOverlay 处理。
var overlayFields = [...]overlaySettingField{
	{key: "content", label: i18n.TUIOverlayContent, read: func(s overlay.Settings) string { return overlayContentLabel(s.Content) }},
	{key: "anchor", label: i18n.TUIOverlayAnchor, read: func(s overlay.Settings) string { return overlayAnchorLabel(string(s.Position.Anchor)) }},
	overlayNumberField("x", i18n.TUIOverlayX, func(s overlay.Settings) float64 { return s.Position.X }, func(s *overlay.Settings, n float64) { s.Position.X = n }),
	overlayNumberField("y", i18n.TUIOverlayY, func(s overlay.Settings) float64 { return s.Position.Y }, func(s *overlay.Settings, n float64) { s.Position.Y = n }),
	overlayNumberField("width", i18n.TUIOverlayWidth, func(s overlay.Settings) float64 { return s.Width }, func(s *overlay.Settings, n float64) { s.Width = n }),
	overlayNumberField("height", i18n.TUIOverlayHeight, func(s overlay.Settings) float64 { return s.Height }, func(s *overlay.Settings, n float64) { s.Height = n }),
	overlayNumberField("padding-top", i18n.TUIOverlayPaddingTop, func(s overlay.Settings) float64 { return s.Padding.Top }, func(s *overlay.Settings, n float64) { s.Padding.Top = n }),
	overlayNumberField("padding-right", i18n.TUIOverlayPaddingRight, func(s overlay.Settings) float64 { return s.Padding.Right }, func(s *overlay.Settings, n float64) { s.Padding.Right = n }),
	overlayNumberField("padding-bottom", i18n.TUIOverlayPaddingBottom, func(s overlay.Settings) float64 { return s.Padding.Bottom }, func(s *overlay.Settings, n float64) { s.Padding.Bottom = n }),
	overlayNumberField("padding-left", i18n.TUIOverlayPaddingLeft, func(s overlay.Settings) float64 { return s.Padding.Left }, func(s *overlay.Settings, n float64) { s.Padding.Left = n }),
	{key: "font-family", label: i18n.TUIOverlayFontFamily, read: func(s overlay.Settings) string { return s.Font.Family }, assign: func(s *overlay.Settings, value string) error { s.Font.Family = value; return nil }},
	overlayNumberField("font-size", i18n.TUIOverlayFontSize, func(s overlay.Settings) float64 { return s.Font.Size }, func(s *overlay.Settings, n float64) { s.Font.Size = n }),
	{key: "font-weight", label: i18n.TUIOverlayFontWeight, read: func(s overlay.Settings) string { return strconv.Itoa(s.Font.Weight) }, assign: func(s *overlay.Settings, value string) (err error) {
		s.Font.Weight, err = strconv.Atoi(value)
		return err
	}},
	{key: "italic", label: i18n.TUIOverlayItalic, read: func(s overlay.Settings) string { return toggleLabel("", s.Font.Italic) }},
	overlayNumberField("text-alpha", i18n.TUIOverlayTextAlpha, func(s overlay.Settings) float64 { return s.TextAlpha }, func(s *overlay.Settings, n float64) { s.TextAlpha = n }),
	overlayNumberField("background-alpha", i18n.TUIOverlayBackgroundAlpha, func(s overlay.Settings) float64 { return s.BackgroundAlpha }, func(s *overlay.Settings, n float64) { s.BackgroundAlpha = n }),
	{key: "display-id", label: i18n.TUIOverlayDisplayID, read: func(s overlay.Settings) string { return strconv.FormatUint(uint64(s.DisplayID), 10) }, assign: func(s *overlay.Settings, value string) error {
		n, err := strconv.ParseUint(value, 10, 32)
		s.DisplayID, s.Output = uint32(n), ""
		return err
	}},
	{key: "output", label: i18n.TUIOverlayOutput, read: func(s overlay.Settings) string { return s.Output }, assign: func(s *overlay.Settings, value string) error { s.Output, s.DisplayID = value, 0; return nil }},
}

func overlayNumberField(key string, label i18n.Key, get func(overlay.Settings) float64, set func(*overlay.Settings, float64)) overlaySettingField {
	return overlaySettingField{
		key: key, label: label,
		read: func(s overlay.Settings) string { return strconv.FormatFloat(get(s), 'f', -1, 64) },
		assign: func(s *overlay.Settings, value string) error {
			n, err := strconv.ParseFloat(value, 64)
			set(s, n)
			return err
		},
	}
}
func overlayContentLabel(value string) string {
	switch value {
	case "danmaku":
		return i18n.T(i18n.TUIOverlayDanmaku)
	case "combined":
		return i18n.T(i18n.TUIOverlayCombined)
	default:
		return i18n.T(i18n.TUIOverlayStatus)
	}
}
func overlayAnchorLabel(value string) string {
	keys := map[string]i18n.Key{"top-left": i18n.TUIOverlayTopLeft, "top": i18n.TUIOverlayTop, "top-right": i18n.TUIOverlayTopRight, "left": i18n.TUIOverlayLeft, "center": i18n.TUIOverlayCenter, "right": i18n.TUIOverlayRight, "bottom-left": i18n.TUIOverlayBottomLeft, "bottom": i18n.TUIOverlayBottom, "bottom-right": i18n.TUIOverlayBottomRight}
	return i18n.T(keys[value])
}
func (m *Model) overlayStateText() string {
	state := "off"
	if m.overlay != nil {
		state = m.overlay.state
	}
	key := i18n.TUIOverlayOff
	switch state {
	case "starting":
		key = i18n.TUIOverlayStarting
	case "running":
		key = i18n.TUIOverlayRunning
	case "stopping":
		key = i18n.TUIOverlayStopping
	case "failed":
		key = i18n.TUIOverlayFailed
	}
	text := fmt.Sprintf(i18n.T(i18n.TUIOverlayState), i18n.T(key))
	if m.overlay != nil && m.overlay.lastError != nil {
		text += fmt.Sprintf(i18n.T(i18n.TUIOverlayLastError), clean(m.overlay.lastError.Error()))
	}
	return text
}
func (m *Model) performOverlay(action string) tea.Cmd {
	s := m.config.Overlay
	switch action {
	case "overlay-toggle":
		s.Enabled = !m.overlayEnabled
		return m.saveOverlay(s, overlayToggleOperation())
	case "overlay-settings":
		m.choices = nil
		for _, f := range overlayFields {
			m.choices = append(m.choices, choice{i18n.T(f.label) + ": " + clean(f.read(s)), f.key})
		}
		m.choices = append(m.choices, choice{i18n.T(i18n.TUIOverlayRestore), "restore"})
		return m.pick("overlay-fields", i18n.T(i18n.TUIOverlaySettings))
	case "overlay-restore":
		defaults := overlay.DefaultSettings()
		defaults.Enabled, defaults.Content = s.Enabled, s.Content
		return m.saveOverlay(defaults, overlayRestoreOperation())
	}
	return nil
}
func (m *Model) chooseOverlay(value string) tea.Cmd {
	s := m.config.Overlay
	switch m.editKind {
	case "overlay-content":
		s.Content = value
		return m.saveOverlay(s, overlayConfigOperation())
	case "overlay-anchor":
		s.Position.Anchor = overlay.Anchor(value)
		return m.saveOverlay(s, overlayConfigOperation())
	case "overlay-fields":
		switch value {
		case "content":
			m.choices = nil
			for _, v := range []string{"danmaku", "status", "combined"} {
				m.choices = append(m.choices, choice{overlayContentLabel(v), v})
			}
			return m.pick("overlay-content", i18n.T(i18n.TUIOverlayContent))
		case "anchor":
			m.choices = nil
			for _, v := range []string{"top-left", "top", "top-right", "left", "center", "right", "bottom-left", "bottom", "bottom-right"} {
				m.choices = append(m.choices, choice{overlayAnchorLabel(v), v})
			}
			return m.pick("overlay-anchor", i18n.T(i18n.TUIOverlayAnchor))
		case "italic":
			s.Font.Italic = !s.Font.Italic
			return m.saveOverlay(s, overlayConfigOperation())
		case "restore":
			return m.confirm(i18n.T(i18n.TUIOverlayRestoreConfirm), "overlay-restore")
		}
		for _, f := range overlayFields {
			if f.key == value {
				return m.form("overlay-"+value, i18n.T(f.label), f.read(s), false)
			}
		}
	}
	return nil
}
func (m *Model) submitOverlay(kind, value string) tea.Cmd {
	s := m.config.Overlay
	key := strings.TrimPrefix(kind, "overlay-")
	for _, field := range overlayFields {
		if field.key != key || field.assign == nil {
			continue
		}
		if err := field.assign(&s, value); err != nil {
			m.status = i18n.T(i18n.TUIOverlayInvalidNumber)
			return nil
		}
		return m.saveOverlay(s, overlayConfigOperation())
	}
	return nil
}
func (m *Model) saveOverlay(settings overlay.Settings, op operation[struct{}]) tea.Cmd {
	s, err := settings.Normalize()
	if err != nil {
		m.status = fmt.Sprintf(i18n.T(i18n.TUIOverlayInvalidSettings), clean(err.Error()))
		return nil
	}
	m.mode = ""
	m.input.SetValue("")
	m.input.Blur()
	return work(m, op, func(ctx context.Context) (struct{}, error) {
		if err := ctx.Err(); err != nil {
			return struct{}{}, err
		}
		// 基于最新快照保存，避免覆盖其他账号或设置变更。
		cfg := m.store.Config()
		cfg.Overlay = s
		return struct{}{}, m.store.SaveConfig(cfg)
	})
}
