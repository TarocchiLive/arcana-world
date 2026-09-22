package tui

import (
	"image/color"
	"strings"

	"charm.land/bubbles/v2/textinput"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// 每个模型独立持有主题，派生样式不会影响其他工作区。
type tuiTheme struct {
	id                                                                         string
	dark                                                                       bool
	canvasColor, surfaceColor, elevatedColor                                   color.Color
	textColor, mutedColor, separatorColor                                      color.Color
	accentColor, positiveColor                                                 color.Color
	accent, muted, textStyle, subtle, positive, selectedStyle, warning, danger lipgloss.Style
	hoverStyle                                                                 lipgloss.Style
}

func themeFor(id string, dark bool) tuiTheme {
	var t tuiTheme
	var warningColor, dangerColor color.Color
	lightDark := lipgloss.LightDark(dark)
	switch id {
	case "midnight":
		t = tuiTheme{id: id, canvasColor: lipgloss.Color("#080C1A"), surfaceColor: lipgloss.Color("#111A30"), elevatedColor: lipgloss.Color("#25365A"), textColor: lipgloss.Color("#E9EFFF"), mutedColor: lipgloss.Color("#94A7CA"), separatorColor: lipgloss.Color("#34476B"), accentColor: lipgloss.Color("#94ADFF"), positiveColor: lipgloss.Color("#76D7C5")}
		warningColor, dangerColor = lipgloss.Color("#F2CF66"), lipgloss.Color("#FF91AA")
	case "nord":
		t = tuiTheme{id: id, canvasColor: lipgloss.Color("#242B36"), surfaceColor: lipgloss.Color("#2E3745"), elevatedColor: lipgloss.Color("#414E62"), textColor: lipgloss.Color("#ECEFF4"), mutedColor: lipgloss.Color("#A8B6C8"), separatorColor: lipgloss.Color("#536278"), accentColor: lipgloss.Color("#88C0D0"), positiveColor: lipgloss.Color("#A3BE8C")}
		warningColor, dangerColor = lipgloss.Color("#EBCB8B"), lipgloss.Color("#E99DA6")
	case "ember":
		t = tuiTheme{id: id, canvasColor: lipgloss.Color("#1A1413"), surfaceColor: lipgloss.Color("#281F1D"), elevatedColor: lipgloss.Color("#49332C"), textColor: lipgloss.Color("#F4E9DC"), mutedColor: lipgloss.Color("#BCA497"), separatorColor: lipgloss.Color("#60483F"), accentColor: lipgloss.Color("#F3A875"), positiveColor: lipgloss.Color("#B7CA91")}
		warningColor, dangerColor = lipgloss.Color("#EBD264"), lipgloss.Color("#F28E91")
	case "paper":
		t = tuiTheme{id: id, canvasColor: lipgloss.Color("#E8E1D5"), surfaceColor: lipgloss.Color("#FAF5EB"), elevatedColor: lipgloss.Color("#E3D9C7"), textColor: lipgloss.Color("#342F29"), mutedColor: lipgloss.Color("#736858"), separatorColor: lipgloss.Color("#BDB09A"), accentColor: lipgloss.Color("#775B98"), positiveColor: lipgloss.Color("#356E58")}
		warningColor, dangerColor = lipgloss.Color("#976900"), lipgloss.Color("#B23D48")
	default:
		t = tuiTheme{
			id:             "lumen",
			canvasColor:    lightDark(lipgloss.Color("#ECEEF4"), lipgloss.Color("#0B0D18")),
			surfaceColor:   lightDark(lipgloss.Color("#FAFBFE"), lipgloss.Color("#15192B")),
			elevatedColor:  lightDark(lipgloss.Color("#E9E2F7"), lipgloss.Color("#2B2544")),
			textColor:      lightDark(lipgloss.Color("#25283B"), lipgloss.Color("#F0F0FF")),
			mutedColor:     lightDark(lipgloss.Color("#626B83"), lipgloss.Color("#929CB8")),
			separatorColor: lightDark(lipgloss.Color("#CDD2E0"), lipgloss.Color("#303852")),
			accentColor:    lightDark(lipgloss.Color("#6944AD"), lipgloss.Color("#AB91FF")),
			positiveColor:  lightDark(lipgloss.Color("#167568"), lipgloss.Color("#65DDD0")),
		}
		warningColor = lightDark(lipgloss.Color("#946C00"), lipgloss.Color("#E8CC69"))
		dangerColor = lightDark(lipgloss.Color("#B33550"), lipgloss.Color("#F08B9C"))
	}
	t.dark = dark
	t.accent = lipgloss.NewStyle().Foreground(t.accentColor).Bold(true)
	t.muted = lipgloss.NewStyle().Foreground(t.mutedColor)
	t.textStyle = lipgloss.NewStyle().Foreground(t.textColor)
	t.subtle = lipgloss.NewStyle().Foreground(t.separatorColor)
	t.positive = lipgloss.NewStyle().Foreground(t.positiveColor)
	t.selectedStyle = lipgloss.NewStyle().Foreground(t.accentColor).Background(t.elevatedColor).Bold(true)
	t.hoverStyle = lipgloss.NewStyle().Foreground(t.accentColor).Background(t.elevatedColor)
	t.warning = lipgloss.NewStyle().Foreground(warningColor).Bold(true)
	t.danger = lipgloss.NewStyle().Foreground(dangerColor).Bold(true)
	return t
}

// 流光只在分隔色与强调色之间轻微混合，明暗主题使用同一规则。
func (t tuiTheme) separatorGlow(strength float64) color.Color {
	r, g, b, _ := t.separatorColor.RGBA()
	ar, ag, ab, _ := t.accentColor.RGBA()
	mix := func(base, accent uint32) uint8 {
		return uint8((float64(base) + (float64(accent)-float64(base))*strength) / 257)
	}
	return color.RGBA{R: mix(r, ar), G: mix(g, ag), B: mix(b, ab), A: 255}
}

func (t tuiTheme) listRow(label string, width int, selected bool) string {
	width = max(1, width)
	label = selectionText(label)
	style := t.textStyle.PaddingLeft(min(3, width-1))
	if selected {
		label = "› " + label
		style = t.selectedStyle.PaddingLeft(min(1, width-1))
	}
	return style.Width(width).MaxWidth(width).Render(ansi.Truncate(label, width-style.GetHorizontalPadding(), "…"))
}

func (t tuiTheme) sectionTitle(title string, width int) string {
	return t.textStyle.Bold(true).Render(ansi.Truncate(selectionText(title), max(1, width), "…"))
}

func (t tuiTheme) hintText(text string, width int) string {
	return t.muted.Render(ansi.Wrap(strings.TrimSpace(clean(text)), max(1, width), ""))
}

func (t tuiTheme) detailRow(label, value string, width int) string {
	width = max(1, width)
	label = strings.TrimSpace(label)
	if width < 36 || ansi.StringWidth(label) > 18 {
		return lipgloss.JoinVertical(lipgloss.Left,
			t.muted.Width(width).Render(label), t.textStyle.Width(width).Render(value))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top,
		t.muted.Width(20).Render(label),
		t.textStyle.Width(width-20).Render(ansi.Wrap(value, width-20, "")))
}

func (t tuiTheme) panel(content string, width, height, paddingX, paddingY, border int, focused bool) string {
	style := lipgloss.NewStyle().Foreground(t.textColor).Background(t.surfaceColor).
		Width(max(1, width)).Height(max(1, height)).
		Padding(paddingY, paddingX).MaxWidth(max(1, width)).MaxHeight(max(1, height))
	if border > 0 {
		color := t.separatorColor
		if focused {
			color = t.accentColor
		}
		style = style.Border(lipgloss.RoundedBorder()).BorderForeground(color).BorderBackground(t.canvasColor)
	}
	return t.paintSurface(style.Render(content), t.surfaceColor)
}

func (t tuiTheme) dim(content string) string {
	return preserveStyle(content, lipgloss.NewStyle().Faint(true))
}

// 子样式重置 ANSI 状态后应恢复所属面板的底色，
// 而不是回到终端默认背景；渲染组件可能使用两种等价的重置序列。
func (t tuiTheme) paintSurface(content string, background color.Color) string {
	return preserveStyle(content, lipgloss.NewStyle().Foreground(t.textColor).Background(background))
}

func preserveStyle(content string, style lipgloss.Style) string {
	sample := style.Render(" ")
	prefix, _, _ := strings.Cut(sample, " ")
	if prefix == "" {
		return content
	}
	content = strings.ReplaceAll(content, ansi.ResetStyle, ansi.ResetStyle+prefix)
	return prefix + strings.ReplaceAll(content, "\x1b[0m", "\x1b[0m"+prefix) + ansi.ResetStyle
}

func (t tuiTheme) styleInput(input *textinput.Model) {
	styles := textinput.DefaultStyles(t.dark)
	styles.Focused = textinput.StyleState{
		Prompt: t.accent, Text: t.textStyle, Placeholder: t.muted, Suggestion: t.muted,
	}
	styles.Blurred = styles.Focused
	styles.Blurred.Prompt = t.muted
	styles.Cursor.Color = t.accentColor
	input.SetStyles(styles)
	input.SetVirtualCursor(false)
}
