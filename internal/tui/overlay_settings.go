package tui

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"arcana-world/internal/i18n"
	"arcana-world/internal/overlay"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

type overlaySettingField struct {
	key    string
	label  i18n.Key
	read   func(overlay.Settings) string
	assign func(*overlay.Settings, string) error
	color  func(*overlay.Colors) *string
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
	{key: "outline", label: i18n.TUIOverlayOutline, read: func(s overlay.Settings) string { return toggleLabel("", s.Outline) }},
	overlayColorField("text-color", i18n.TUIOverlayTextColor, func(c *overlay.Colors) *string { return &c.Text }),
	overlayColorField("background-color", i18n.TUIOverlayBackgroundColor, func(c *overlay.Colors) *string { return &c.Background }),
	overlayColorField("captain-color", i18n.TUIOverlayCaptainColor, func(c *overlay.Colors) *string { return &c.Captain }),
	overlayColorField("admiral-color", i18n.TUIOverlayAdmiralColor, func(c *overlay.Colors) *string { return &c.Admiral }),
	overlayColorField("governor-color", i18n.TUIOverlayGovernorColor, func(c *overlay.Colors) *string { return &c.Governor }),
	overlayColorField("super-chat-color", i18n.TUIOverlaySuperChatColor, func(c *overlay.Colors) *string { return &c.SuperChat }),
	overlayNumberField("text-alpha", i18n.TUIOverlayTextAlpha, func(s overlay.Settings) float64 { return s.TextAlpha }, func(s *overlay.Settings, n float64) { s.TextAlpha = n }),
	overlayNumberField("background-alpha", i18n.TUIOverlayBackgroundAlpha, func(s overlay.Settings) float64 { return s.BackgroundAlpha }, func(s *overlay.Settings, n float64) { s.BackgroundAlpha = n }),
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

func overlayColorField(key string, label i18n.Key, color func(*overlay.Colors) *string) overlaySettingField {
	return overlaySettingField{
		key: key, label: label, color: color,
		read: func(s overlay.Settings) string {
			colors, _ := s.Colors.Normalize()
			return *color(&colors)
		},
		assign: func(s *overlay.Settings, value string) error {
			if _, err := overlay.ParseColor(value); err != nil {
				return err
			}
			*color(&s.Colors) = strings.ToUpper(value)
			return nil
		},
	}
}

func (m *Model) editingOverlayColor() *overlaySettingField {
	if m.mode != "form" {
		return nil
	}
	for i := range overlayFields {
		f := &overlayFields[i]
		if f.color != nil && m.editKind == "overlay-"+f.key {
			return f
		}
	}
	return nil
}

func (m *Model) updateOverlayColorPreview() {
	field := m.editingOverlayColor()
	if field == nil {
		return
	}
	value := m.input.Value()
	if _, err := overlay.ParseColor(value); err != nil {
		return
	}
	colors, _ := m.config.Overlay.Colors.Normalize()
	*field.color(&colors) = strings.ToUpper(value)
	if m.overlayColorPreview != nil && *m.overlayColorPreview == colors {
		return
	}
	m.overlayColorPreview = &colors
	m.publishOverlay()
}

func (m *Model) previewOverlayColors(cfg overlay.Config) overlay.Config {
	if m.overlayColorPreview != nil {
		cfg.Colors = *m.overlayColorPreview
	}
	return cfg
}

func (m *Model) clearOverlayColorPreview() {
	if m.overlayColorPreview != nil {
		m.overlayColorPreview = nil
		m.publishOverlay()
	}
}

func (m *Model) overlayColorSample() string {
	field := m.editingOverlayColor()
	if field == nil {
		return ""
	}
	colors, _ := m.config.Overlay.Colors.Normalize()
	if m.overlayColorPreview != nil {
		colors = *m.overlayColorPreview
	}
	color := *field.color(&colors)
	text := color + "  " + i18n.T(i18n.TUIOverlayColorSample)
	foreground := "#FFFFFF"
	rgb, _ := overlay.ParseColor(color)
	if 299*((rgb>>16)&255)+587*((rgb>>8)&255)+114*(rgb&255) >= 128000 {
		foreground = "#000000"
	}
	swatch := lipgloss.NewStyle().Background(lipgloss.Color(color)).Render("    ")
	sample := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Background(lipgloss.Color(foreground)).Render(" " + text + " ")
	if field.key == "background-color" {
		sample = lipgloss.NewStyle().Foreground(lipgloss.Color(foreground)).Background(lipgloss.Color(color)).Render(" " + text + " ")
	}
	return "\n\n" + swatch + "  " + sample
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

type overlaySettingsNavigation struct {
	selected int
	offset   int
}

func (m *Model) showOverlaySettings() tea.Cmd {
	m.input.SetValue("")
	m.input.Blur()
	m.choices = nil
	for _, field := range overlayFields {
		m.choices = append(m.choices, choice{i18n.T(field.label) + ": " + clean(field.read(m.config.Overlay)), field.key})
	}
	m.choices = append(m.choices, choice{i18n.T(i18n.TUIOverlayRestore), "restore"})
	cmd := m.pick("overlay-fields", i18n.T(i18n.TUIOverlaySettings))
	if position := m.overlaySettings; position != nil {
		m.selected = min(position.selected, len(m.choices)-1)
		m.view.SetContent(m.content())
		m.view.SetYOffset(position.offset)
	}
	return cmd
}

func (m *Model) performOverlay(action string) tea.Cmd {
	s := m.config.Overlay
	switch action {
	case "overlay-toggle":
		s.Enabled = !m.overlayEnabled
		return m.saveOverlay(s, overlayToggleOperation())
	case "overlay-displays":
		return m.pickOverlayDisplays()
	case "overlay-settings":
		m.overlaySettings = &overlaySettingsNavigation{}
		return m.showOverlaySettings()
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
		if m.overlaySettings != nil {
			m.overlaySettings.selected, m.overlaySettings.offset = m.selected, m.view.YOffset()
		}
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
		case "outline":
			s.Outline = !s.Outline
			return m.saveOverlay(s, overlayConfigOperation())
		case "restore":
			return m.confirm(i18n.T(i18n.TUIOverlayRestoreConfirm), "overlay-restore")
		}
		for _, f := range overlayFields {
			if f.key == value {
				cmd := m.form("overlay-"+value, i18n.T(f.label), f.read(s), false)
				m.updateOverlayColorPreview()
				return cmd
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
			m.warnStatus(i18n.T(i18n.TUIOverlayInvalidNumber))
			if field.color != nil {
				m.warnStatus(i18n.T(i18n.TUIOverlayInvalidColor))
			}
			return nil
		}
		return m.saveOverlay(s, overlayConfigOperation())
	}
	return nil
}
func (m *Model) saveOverlay(settings overlay.Settings, op operation[struct{}]) tea.Cmd {
	s, err := settings.Normalize()
	if err != nil {
		m.warnStatus(fmt.Sprintf(i18n.T(i18n.TUIOverlayInvalidSettings), clean(err.Error())))
		return nil
	}
	m.mode = ""
	m.input.SetValue("")
	m.input.Blur()
	if m.overlaySettings != nil {
		m.showOverlaySettings()
	}
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
