//go:build windows && (386 || amd64 || arm64)

package native

import (
	"arcana-world/internal/overlay"
	"fmt"
	"math"
	"runtime"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 原生线程拥有 DC 及其选入的位图/字体。pixels 是 DIB
// 分配区域的别名，替换或清理后不得继续保留。
// DC 和选入对象在文本更新之间持续存在。替换采用
// 事务方式：先选入新对象，再删除旧的自有对象。
func (state *winState) prepareSurface(size winSize, fontHeight int32) error {
	if state.dc == 0 {
		dc, _, err := winCreateDC.Call(0)
		if dc == 0 {
			return winError("CreateCompatibleDC", err)
		}
		state.dc = dc
	}
	if state.bitmap == 0 || state.size != size {
		info := winBitmapInfo{Size: 40, Width: size.Width, Height: -size.Height, Planes: 1, BitCount: 32}
		var bits unsafe.Pointer
		bitmap, _, err := winCreateDIB.Call(state.dc, uintptr(unsafe.Pointer(&info)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
		if bitmap == 0 {
			return winError("CreateDIBSection", err)
		}
		if bits == nil {
			winDeleteObject.Call(bitmap)
			return fmt.Errorf("overlay: CreateDIBSection returned no pixel buffer")
		}
		previous, _, err := winSelectObject.Call(state.dc, bitmap)
		if previous == 0 || previous == ^uintptr(0) {
			winDeleteObject.Call(bitmap)
			return winError("SelectObject (bitmap)", err)
		}
		if state.bitmap == 0 {
			state.originalBitmap = previous
		} else {
			winDeleteObject.Call(state.bitmap)
		}
		state.bitmap, state.size = bitmap, size
		state.pixels = unsafe.Slice((*byte)(bits), int(size.Width)*int(size.Height)*4)
	}
	if state.font == 0 || state.fontHeight != fontHeight || state.fontConfig != state.cfg.Font {
		family := state.cfg.Font.Family
		if family == "" {
			family = "Segoe UI"
		}
		face, err := windows.UTF16FromString(family)
		if err != nil {
			return fmt.Errorf("overlay: Windows font family: %w", err)
		}
		// LOGFONT 的名称字段包括终止符，拒绝截断而不是静默选错字体。
		if len(face) > 32 {
			return fmt.Errorf("overlay: Windows font family exceeds 31 UTF-16 code units")
		}
		var italic uintptr
		if state.cfg.Font.Italic {
			italic = 1
		}
		// GDI 字体只提供随 DPI 缩放的固定行距；
		// 文字整形、字体回退和抗锯齿由 DirectWrite / Direct2D 处理。
		font, _, err := winCreateFont.Call(uintptr(-fontHeight), 0, 0, 0, uintptr(state.cfg.Font.Weight), italic, 0, 0, 1, 0, 0, 4, 0, uintptr(unsafe.Pointer(&face[0])))
		runtime.KeepAlive(face)
		if font == 0 {
			return winError("CreateFontW", err)
		}
		previous, _, err := winSelectObject.Call(state.dc, font)
		if previous == 0 || previous == ^uintptr(0) {
			winDeleteObject.Call(font)
			return winError("SelectObject (font)", err)
		}
		if state.font == 0 {
			state.originalFont = previous
		} else {
			winDeleteObject.Call(state.font)
		}
		state.font, state.fontHeight = font, fontHeight
		state.fontConfig = state.cfg.Font
	}
	return nil
}

func (state *winState) closeResources() {
	if state.textLayout != nil {
		state.textLayout.release()
		state.textLayout = nil
	}
	for _, brush := range state.colorBrushes {
		brush.release()
	}
	state.colorBrushes = nil
	state.coverage = nil
	state.outline, state.outlineScratch = nil, nil
	for _, object := range [...]*winCOMObject{state.textBrush, state.drawTarget, state.drawFactory, state.writeFactory} {
		if object != nil {
			object.release()
		}
	}
	state.textBrush, state.drawTarget, state.drawFactory, state.writeFactory = nil, nil, nil, nil
	if state.dc == 0 {
		return
	}
	winGDIFlush.Call()
	if state.originalFont != 0 {
		winSelectObject.Call(state.dc, state.originalFont)
	}
	if state.originalBitmap != 0 {
		winSelectObject.Call(state.dc, state.originalBitmap)
	}
	// 若原生取消选入失败，先删除 DC 再删除其自有对象，作为最后一道保护；
	// 删除时对象不得仍处于选入状态。
	winDeleteDC.Call(state.dc)
	if state.font != 0 {
		winDeleteObject.Call(state.font)
	}
	if state.bitmap != 0 {
		winDeleteObject.Call(state.bitmap)
	}
	state.dc, state.font, state.bitmap = 0, 0, 0
	state.pixels = nil
}

// 随字形掩码缓存半个逻辑点宽的描边，不因颜色变化而更新。
func (state *winState) updateTextOutline(rect winRect) {
	for _, mask := range [...]*[]byte{&state.outline, &state.outlineScratch} {
		if cap(*mask) < len(state.coverage) {
			*mask = make([]byte, len(state.coverage))
		} else {
			*mask = (*mask)[:len(state.coverage)]
			clear(*mask)
		}
	}
	width, height := int(state.size.Width), int(state.size.Height)
	left, right := max(0, int(rect.Left)), min(width, int(rect.Right))
	top, bottom := max(0, int(rect.Top)), min(height, int(rect.Bottom))
	radius := state.outlineRadius
	// 两遍有界最大值滤波使掩码保持与文字颜色无关。
	for y := top; y < bottom; y++ {
		row := y * width
		for x := left; x < right; x++ {
			var value byte
			for nx := max(left, x-radius); nx < min(right, x+radius+1); nx++ {
				value = max(value, state.coverage[row+nx])
			}
			state.outlineScratch[row+x] = value
		}
	}
	for y := top; y < bottom; y++ {
		for x := left; x < right; x++ {
			var value byte
			for ny := max(top, y-radius); ny < min(bottom, y+radius+1); ny++ {
				value = max(value, state.outlineScratch[ny*width+x])
			}
			state.outline[y*width+x] = value
		}
	}
}

func (state *winState) render() error {
	state.dirty = false
	if state.geometryDirty {
		if err := state.refreshGeometry(); err != nil {
			return err
		}
	}
	if state.monitor == 0 {
		return nil
	}
	// 仅移动窗口时复用已提交的像素，不重新整形文字、扫描 alpha 或上传位图。
	prior := state.paintedConfig
	if state.painted && prior.Text == state.cfg.Text && prior.Font == state.cfg.Font &&
		prior.TextRoles == state.cfg.TextRoles && prior.Colors == state.cfg.Colors && prior.Outline == state.cfg.Outline &&
		prior.TextAlpha == state.cfg.TextAlpha && prior.BackgroundAlpha == state.cfg.BackgroundAlpha &&
		state.paintedSize == state.size && state.paintedRect == state.textRect && state.paintedFontHeight == state.fontHeight {
		ok, _, err := winSetWindowPos.Call(state.window, ^uintptr(0), uintptr(state.position.X), uintptr(state.position.Y), 0, 0, 0x0051)
		if ok == 0 {
			return winError("SetWindowPos (move)", err)
		}
		return nil
	}
	if state.encodedSource != state.cfg.Text {
		state.textBuffer = state.textBuffer[:0]
		for _, value := range state.cfg.Text {
			state.textBuffer = utf16.AppendRune(state.textBuffer, value)
		}
		state.encodedSource = state.cfg.Text
	}
	text, pixels, dc, rect := state.textBuffer, state.pixels, state.dc, state.textRect
	clear(pixels)
	hasText := len(text) > 0 && rect.Right > rect.Left && rect.Bottom > rect.Top
	if hasText {
		if err := state.drawTextLines(text, rect); err != nil {
			return err
		}
	}
	if result, _, err := winGDIFlush.Call(); result == 0 {
		return winError("GdiFlush", err)
	}
	// 独立的白色掩码也能为黑色字形提供覆盖率。
	// 彩色绘制阶段提供的 RGB 已乘以字形覆盖率。
	backgroundRGB, err := overlay.ParseColor(state.cfg.Colors.Background)
	if err != nil {
		return err
	}
	background := uint32(math.Round(state.cfg.BackgroundAlpha * 255))
	foreground := uint32(math.Round(state.cfg.TextAlpha * 255))
	backgroundChannels := [3]uint32{backgroundRGB & 255, backgroundRGB >> 8 & 255, backgroundRGB >> 16 & 255}
	outlineChannels := [3]uint32{24, 19, 16}
	for i := 0; i < len(pixels); i += 4 {
		var coverage, outlined uint32
		if hasText && state.coverageValid {
			coverage = uint32(state.coverage[i/4])
			outlined = coverage
			if state.cfg.Outline {
				outlined = uint32(state.outline[i/4])
			}
		}
		value := (coverage*foreground + 127) / 255
		total := (outlined*foreground + 127) / 255
		edge := total - value
		behind := (background*(255-total) + 127) / 255
		alpha := total + behind
		for channel, rgb := range backgroundChannels {
			color := (uint32(pixels[i+channel])*foreground + 127) / 255
			pixels[i+channel] = byte(min(alpha, color+(outlineChannels[channel]*edge+127)/255+(rgb*behind+127)/255))
		}
		pixels[i+3] = byte(alpha)
	}
	destination, source := state.position, winPoint{}
	size := state.size
	blend := winBlend{ConstantAlpha: 255, AlphaFormat: 1}
	ok, _, err := winUpdateLayered.Call(state.window, 0, uintptr(unsafe.Pointer(&destination)), uintptr(unsafe.Pointer(&size)), dc, uintptr(unsafe.Pointer(&source)), 0, uintptr(unsafe.Pointer(&blend)), 2)
	if ok == 0 {
		return winError("UpdateLayeredWindow", err)
	}
	// HWND_TOPMOST，带有 SWP_NOACTIVATE|NOMOVE|NOSIZE|SHOWWINDOW。重新设置
	// 该置顶层级不会授予键盘焦点，包括文本更新期间。
	ok, _, err = winSetWindowPos.Call(state.window, ^uintptr(0), 0, 0, 0, 0, 0x0053)
	if ok == 0 {
		return winError("SetWindowPos (show)", err)
	}
	state.painted = true
	state.paintedConfig, state.paintedSize = state.cfg, state.size
	state.paintedRect, state.paintedFontHeight = state.textRect, state.fontHeight
	return nil
}
