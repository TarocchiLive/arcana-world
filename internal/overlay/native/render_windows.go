//go:build windows

package native

import (
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
		if result, _, err := winSetTextColor.Call(dc, 0xFFFFFF); result == 0xFFFFFFFF {
			return winError("SetTextColor", err)
		}
		if result, _, err := winSetBackgroundColor.Call(dc, 0); result == 0xFFFFFFFF {
			return winError("SetBkColor", err)
		}
		if result, _, err := winSetBackgroundMode.Call(dc, 1); result == 0 {
			return winError("SetBkMode", err)
		}
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
		// ANTIALIASED_QUALITY 可避免半透明表面出现次像素 ClearType 条纹。
		// DrawText 使用 Windows 字体链接实现 Unicode 回退；
		// 可用字形仍取决于已安装的字体。
		// Wine 的 CJK 缺字方框问题通过在
		// 测试前缀中安装/链接 Noto CJK 解决，而不是替换为其他应用渲染器。
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
	if len(text) > 0 && rect.Right > rect.Left && rect.Bottom > rect.Top {
		// DT_WORDBREAK | DT_NOPREFIX | DT_EDITCONTROL：保留与号，
		// 处理原生 Unicode 字体整形、换行和裁剪，而非调整大小。
		result, _, err := winDrawText.Call(dc, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)), uintptr(unsafe.Pointer(&rect)), 0x2850)
		runtime.KeepAlive(text)
		if result == 0 {
			return winError("DrawTextW", err)
		}
	}
	if result, _, err := winGDIFlush.Call(); result == 0 {
		return winError("GdiFlush", err)
	}
	// GDI 不会写入有意义的 alpha。其黑底白字 RGB 是覆盖率
	// 掩码。在预乘 BGRA 中，将白色文本合成到半透明黑色上：
	// rgb = coverage*textAlpha; a = rgb + backgroundAlpha*(1-rgb).
	// 要显示此 alpha，Wine 需要 X 合成器：缺少 xcompmgr 时，
	// 即使像素正确也会产生黑色矩形。该环境问题
	// 不是将原生面板设为不透明或削弱点击穿透的理由。
	background := uint32(math.Round(state.cfg.BackgroundAlpha * 255))
	foreground := uint32(math.Round(state.cfg.TextAlpha * 255))
	for i := 0; i < len(pixels); i += 4 {
		coverage := max(pixels[i], pixels[i+1], pixels[i+2])
		value := (uint32(coverage)*foreground + 127) / 255
		alpha := value + (background*(255-value)+127)/255
		pixels[i], pixels[i+1], pixels[i+2], pixels[i+3] = byte(value), byte(value), byte(value), byte(alpha)
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
	if state.ready != nil {
		state.ready()
		state.ready = nil
	}
	return nil
}
