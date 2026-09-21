package tui

import (
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

const (
	showcaseFrames   = 180
	showcaseInterval = time.Second / 10
)

type showcaseState struct {
	generation uint64
	frame      int
	running    bool
}

type showcaseTick struct{ generation uint64 }

func (m *Model) showcaseVisible() bool {
	return m.width >= 100 && m.height >= 28 && !m.config.TUICompactHeader
}

func (m *Model) syncShowcase() tea.Cmd {
	// 外观尺寸与运行状态分离，弹窗和忙碌状态不会挤动正文。
	running := m.initialized && !m.blurred && m.showcaseVisible() && !m.config.TUIMotionDisabled && m.mode != "confirm"
	if running == m.showcase.running {
		return nil
	}
	m.showcase.generation++
	m.showcase.running = running
	if !running {
		return nil
	}
	return nextShowcaseTick(m.showcase.generation)
}

func (m *Model) updateShowcase(msg showcaseTick) tea.Cmd {
	if !m.showcase.running || msg.generation != m.showcase.generation {
		return nil
	}
	if !m.initialized || m.blurred || !m.showcaseVisible() || m.config.TUIMotionDisabled || m.mode == "confirm" {
		return m.syncShowcase()
	}
	m.showcase.frame = (m.showcase.frame + 1) % showcaseFrames
	return nextShowcaseTick(m.showcase.generation)
}

func nextShowcaseTick(generation uint64) tea.Cmd {
	return tea.Tick(showcaseInterval, func(time.Time) tea.Msg {
		return showcaseTick{generation: generation}
	})
}

// 分隔线位于所有命中区域和通知上方，只合成这一行，保留正文与悬浮状态。
func (m *Model) refreshShowcaseFrame() {
	frame := &m.frame
	if !frame.separatorDirty || frame.separator.width == 0 {
		return
	}
	line := frame.separator
	line.content = m.theme.paintSurface(m.showcaseSeparator(line.width), m.theme.canvasColor)
	frame.base = composeRow(frame.base, line)
	frame.view.Content = ""
	frame.separatorDirty = false
}

// 小标题与宽字标形成层级；字形保持静止，仅细分隔线承载流光。
func (m *Model) wordmark() string {
	return lipgloss.JoinVertical(lipgloss.Left,
		m.theme.accent.Render("Arcana"),
		m.theme.textStyle.Render("╷   ╷  ╭───╮  ╭───╮  ╷      ┌───╮"),
		m.theme.textStyle.Render("│ ╷ │  │   │  ├─┬─╯  │      │   │"),
		m.theme.textStyle.Render("╰─┴─╯  ╰───╯  ╵ ╰─╴  ╰───╴  └───╯"))
}

func (m *Model) showcaseSeparator(width int) string {
	width = max(1, width)
	if m.config.TUIMotionDisabled {
		return m.theme.subtle.Render(strings.Repeat("─", width))
	}
	// 十八秒完成一次往返，余弦缓入缓出；高光仅覆盖少量字符。
	phase := float64(m.showcase.frame) * 2 * math.Pi / showcaseFrames
	center := (1 - math.Cos(phase)) * 0.5 * float64(width-1)
	start := max(0, int(math.Ceil(center-6)))
	end := min(width, int(math.Floor(center+6))+1)
	var out strings.Builder
	out.WriteString(m.theme.subtle.Render(strings.Repeat("─", start)))
	for x := start; x < end; x++ {
		strength := math.Max(0, 1-math.Abs(float64(x)-center)/6) * 0.3
		out.WriteString(lipgloss.NewStyle().Foreground(m.theme.separatorGlow(strength)).Render("─"))
	}
	out.WriteString(m.theme.subtle.Render(strings.Repeat("─", width-end)))
	return out.String()
}
