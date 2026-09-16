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
	key   string
	label i18n.Key
	value string
}

func overlayFields(s overlay.Settings) []overlaySettingField {
	n := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	return []overlaySettingField{
		{"content", i18n.TUIOverlayContent, overlayContentLabel(s.Content)},
		{"anchor", i18n.TUIOverlayAnchor, overlayAnchorLabel(string(s.Position.Anchor))},
		{"x", i18n.TUIOverlayX, n(s.Position.X)}, {"y", i18n.TUIOverlayY, n(s.Position.Y)},
		{"width", i18n.TUIOverlayWidth, n(s.Width)}, {"height", i18n.TUIOverlayHeight, n(s.Height)},
		{"padding-top", i18n.TUIOverlayPaddingTop, n(s.Padding.Top)}, {"padding-right", i18n.TUIOverlayPaddingRight, n(s.Padding.Right)},
		{"padding-bottom", i18n.TUIOverlayPaddingBottom, n(s.Padding.Bottom)}, {"padding-left", i18n.TUIOverlayPaddingLeft, n(s.Padding.Left)},
		{"font-family", i18n.TUIOverlayFontFamily, s.Font.Family}, {"font-size", i18n.TUIOverlayFontSize, n(s.Font.Size)},
		{"font-weight", i18n.TUIOverlayFontWeight, strconv.Itoa(s.Font.Weight)}, {"italic", i18n.TUIOverlayItalic, toggleLabel("", s.Font.Italic)},
		{"text-alpha", i18n.TUIOverlayTextAlpha, n(s.TextAlpha)}, {"background-alpha", i18n.TUIOverlayBackgroundAlpha, n(s.BackgroundAlpha)},
		{"display-id", i18n.TUIOverlayDisplayID, strconv.FormatUint(uint64(s.DisplayID), 10)}, {"output", i18n.TUIOverlayOutput, s.Output},
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
		return m.saveOverlay(s, "overlay-toggle")
	case "overlay-settings":
		m.choices = nil
		for _, f := range overlayFields(s) {
			m.choices = append(m.choices, choice{i18n.T(f.label) + ": " + clean(f.value), f.key})
		}
		m.choices = append(m.choices, choice{i18n.T(i18n.TUIOverlayRestore), "restore"})
		return m.pick("overlay-fields", i18n.T(i18n.TUIOverlaySettings))
	case "overlay-restore":
		defaults := overlay.DefaultSettings()
		defaults.Enabled, defaults.Content = s.Enabled, s.Content
		return m.saveOverlay(defaults, "overlay-restore")
	}
	return nil
}
func (m *Model) chooseOverlay(value string) tea.Cmd {
	s := m.config.Overlay
	switch m.editKind {
	case "overlay-content":
		s.Content = value
		return m.saveOverlay(s, "overlay-config")
	case "overlay-anchor":
		s.Position.Anchor = overlay.Anchor(value)
		return m.saveOverlay(s, "overlay-config")
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
			return m.saveOverlay(s, "overlay-config")
		case "restore":
			return m.confirm(i18n.T(i18n.TUIOverlayRestoreConfirm), "overlay-restore")
		}
		for _, f := range overlayFields(s) {
			if f.key == value {
				return m.form("overlay-"+value, i18n.T(f.label), f.value, false)
			}
		}
	}
	return nil
}
func (m *Model) submitOverlay(kind, value string) tea.Cmd {
	s := m.config.Overlay
	var err error
	switch strings.TrimPrefix(kind, "overlay-") {
	case "font-family":
		s.Font.Family = value
	case "output":
		s.Output = value
		s.DisplayID = 0
	case "display-id":
		var n uint64
		n, err = strconv.ParseUint(value, 10, 32)
		s.DisplayID, s.Output = uint32(n), ""
	case "font-weight":
		s.Font.Weight, err = strconv.Atoi(value)
	default:
		var target *float64
		switch kind {
		case "overlay-x":
			target = &s.Position.X
		case "overlay-y":
			target = &s.Position.Y
		case "overlay-width":
			target = &s.Width
		case "overlay-height":
			target = &s.Height
		case "overlay-padding-top":
			target = &s.Padding.Top
		case "overlay-padding-right":
			target = &s.Padding.Right
		case "overlay-padding-bottom":
			target = &s.Padding.Bottom
		case "overlay-padding-left":
			target = &s.Padding.Left
		case "overlay-font-size":
			target = &s.Font.Size
		case "overlay-text-alpha":
			target = &s.TextAlpha
		case "overlay-background-alpha":
			target = &s.BackgroundAlpha
		}
		if target == nil {
			return nil
		}
		*target, err = strconv.ParseFloat(value, 64)
	}
	if err != nil {
		m.status = i18n.T(i18n.TUIOverlayInvalidNumber)
		return nil
	}
	return m.saveOverlay(s, "overlay-config")
}
func (m *Model) saveOverlay(settings overlay.Settings, kind string) tea.Cmd {
	s, err := settings.Normalize()
	if err != nil {
		m.status = fmt.Sprintf(i18n.T(i18n.TUIOverlayInvalidSettings), clean(err.Error()))
		return nil
	}
	m.mode = ""
	m.input.SetValue("")
	m.input.Blur()
	return m.work(kind, func(ctx context.Context) (any, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// Read the latest store snapshot rather than overwriting unrelated account/settings changes.
		cfg := m.store.Config()
		cfg.Overlay = s
		return nil, m.store.SaveConfig(cfg)
	})
}
