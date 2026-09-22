package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// floatingLayer 使用终端单元格记录浮层的位置和尺寸，边框也计入尺寸。
type floatingLayer struct {
	content             string
	x, y, width, height int
}

// composeLayer 由官方画布处理叠放、裁剪、宽字符和 ANSI 样式，不扩展底层画布。
func composeLayer(base string, layers ...floatingLayer) string {
	// 画布输出的每行独立恢复样式，便于后续只替换悬浮行而不影响邻行。
	visible := make([]*lipgloss.Layer, 1, len(layers)+1)
	visible[0] = lipgloss.NewLayer(base)
	for _, layer := range layers {
		if layer.content == "" || layer.width <= 0 || layer.height <= 0 {
			continue
		}
		visible = append(visible, lipgloss.NewLayer(layer.content).X(layer.x).Y(layer.y).Z(len(visible)))
	}
	width, height := lipgloss.Size(base)
	return lipgloss.NewCanvas(width, height).Compose(lipgloss.NewCompositor(visible...)).Render()
}

// 单行更新仍由画布处理 ANSI 和宽字符，其余行直接复用。
func composeRow(base string, layer floatingLayer) string {
	start := 0
	for range layer.y {
		next := strings.IndexByte(base[start:], '\n')
		if next < 0 {
			return base
		}
		start += next + 1
	}
	end := len(base)
	if next := strings.IndexByte(base[start:], '\n'); next >= 0 {
		end = start + next
	}
	layer.y = 0
	row := composeLayer(base[start:end], layer)
	return base[:start] + row + base[end:]
}
