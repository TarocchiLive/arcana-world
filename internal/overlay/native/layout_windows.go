//go:build windows && (386 || amd64 || arm64)

package native

import (
	"fmt"
	"math"
	"runtime"
	"syscall"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	winDWriteCreateFactory = windows.NewLazySystemDLL("dwrite.dll").NewProc("DWriteCreateFactory")
	winD2DCreateFactory    = windows.NewLazySystemDLL("d2d1.dll").NewProc("D2D1CreateFactory")
	winGetTextMetrics      = winGDI.NewProc("GetTextMetricsW")
	winDWriteFactoryIID    = windows.GUID{Data1: 0xb859ee5a, Data2: 0xd838, Data3: 0x4b5b, Data4: [8]byte{0xa2, 0xe8, 0x1a, 0xdc, 0x7d, 0x93, 0xdb, 0x48}}
	winD2DFactoryIID       = windows.GUID{Data1: 0x06152247, Data2: 0x6f50, Data3: 0x465a, Data4: [8]byte{0x92, 0x45, 0x11, 0x8b, 0xfd, 0x3b, 0x60, 0x07}}
)

// TEXTMETRICW 包含十一个 LONG、四个 WCHAR 和五个 BYTE。
// GDI 返回整数物理像素，Height 可直接用作固定行槽高度。
type winTextMetrics struct {
	Height, Ascent, Descent, InternalLeading, ExternalLeading      int32
	AverageWidth, MaximumWidth, Weight, Overhang, AspectX, AspectY int32
	First, Last, Default, Break                                    uint16
	Italic, Underlined, StruckOut, PitchAndFamily, Charset         byte
}

type winD2DProperties struct {
	Type, Format, AlphaMode uint32
	DPIX, DPIY              float32
	Usage, MinLevel         uint32
}

type winLineMetrics struct {
	Length, TrailingWhitespaceLength, NewlineLength uint32
	Height, Baseline                                float32
	IsTrimmed                                       int32
}

type winTextRange struct{ Start, Length uint32 }

type winPointF struct{ X, Y float32 }

type winCOMObject struct {
	vtable *[64]uintptr
}

// Release 只有指针参数，无需浮点或结构体 ABI 适配。
func (object *winCOMObject) release() {
	syscall.SyscallN(object.vtable[2], uintptr(unsafe.Pointer(object)))
}

type winMethod[F any] struct {
	address uintptr
	call    F
}

// 仅原生线程访问；按实际方法地址缓存，避免逐帧注册，也不假定不同对象共用虚表。
func (method *winMethod[F]) bind(object *winCOMObject, slot int) F {
	address := object.vtable[slot]
	if method.address != address {
		winRegisterMethod(&method.call, address)
		method.address = address
	}
	return method.call
}

type winTextMethods struct {
	createFormat  winMethod[func(*winCOMObject, *uint16, *winCOMObject, uint32, uint32, uint32, float32, *uint16, **winCOMObject) int32]
	createLayout  winMethod[func(*winCOMObject, *uint16, uint32, *winCOMObject, float32, float32, **winCOMObject) int32]
	lineSpacing   winMethod[func(*winCOMObject, uint32, float32, float32) int32]
	wordWrapping  winMethod[func(*winCOMObject, uint32) int32]
	lineMetrics   winMethod[func(*winCOMObject, *winLineMetrics, uint32, *uint32) int32]
	drawingEffect winMethod[func(*winCOMObject, *winCOMObject, winTextRange) int32]
	createTarget  winMethod[func(*winCOMObject, *winD2DProperties, **winCOMObject) int32]
	bindDC        winMethod[func(*winCOMObject, uintptr, *winRect) int32]
	createBrush   winMethod[func(*winCOMObject, *[4]float32, unsafe.Pointer, **winCOMObject) int32]
	antialias     winMethod[func(*winCOMObject, uint32)]
	transform     winMethod[func(*winCOMObject, *[6]float32)]
	beginDraw     winMethod[func(*winCOMObject)]
	clear         winMethod[func(*winCOMObject, *[4]float32)]
	pushClip      winMethod[func(*winCOMObject, *[4]float32, uint32)]
	drawLayout    winMethod[func(*winCOMObject, winPointF, *winCOMObject, *winCOMObject, uint32)]
	popClip       winMethod[func(*winCOMObject)]
	endDraw       winMethod[func(*winCOMObject, *uint64, *uint64) int32]
}

func winHRESULT(operation string, result int32) error {
	if result < 0 {
		return fmt.Errorf("overlay: %s: HRESULT 0x%08x", operation, uint32(result))
	}
	return nil
}

// 保留整段布局，由 DirectWrite 决定视觉换行（包括长词的字形簇安全断行）。
// 使用 GDI 固定行槽平移并裁剪完整尾行，不按消息边界或 UTF-16 子串重新排版。
func (state *winState) drawTextLines(text []uint16, rect winRect) error {
	api := &state.textMethods
	if state.writeFactory == nil {
		hr, _, _ := winDWriteCreateFactory.Call(0, uintptr(unsafe.Pointer(&winDWriteFactoryIID)), uintptr(unsafe.Pointer(&state.writeFactory)))
		if err := winHRESULT("DWriteCreateFactory", int32(hr)); err != nil {
			return err
		}
	}
	width := rect.Right - rect.Left
	if state.textLayout == nil || state.layoutSource != state.cfg.Text ||
		state.layoutFont != state.cfg.Font || state.layoutFontHeight != state.fontHeight || state.layoutWidth != width {
		var metrics winTextMetrics
		if ok, _, err := winGetTextMetrics.Call(state.dc, uintptr(unsafe.Pointer(&metrics))); ok == 0 {
			return winError("GetTextMetricsW", err)
		}
		if metrics.Height <= 0 || metrics.Ascent <= 0 {
			return fmt.Errorf("overlay: Windows font returned invalid line metrics")
		}
		family := state.cfg.Font.Family
		if family == "" {
			family = "Segoe UI"
		}
		face, err := windows.UTF16FromString(family)
		if err != nil {
			return fmt.Errorf("overlay: Windows font family: %w", err)
		}
		locale := [1]uint16{}
		var format, layout *winCOMObject
		var style uint32
		if state.cfg.Font.Italic {
			style = 2
		}
		factory := state.writeFactory
		if err := winHRESULT("IDWriteFactory.CreateTextFormat", api.createFormat.bind(factory, 15)(factory,
			&face[0], nil, uint32(state.cfg.Font.Weight), style, 5, float32(state.fontHeight), &locale[0], &format)); err != nil {
			return err
		}
		defer format.release()
		if err := winHRESULT("IDWriteTextFormat.SetLineSpacing",
			api.lineSpacing.bind(format, 10)(format, 1, float32(metrics.Height), float32(metrics.Ascent))); err != nil {
			return err
		}
		if err := winHRESULT("IDWriteTextFormat.SetWordWrapping", api.wordWrapping.bind(format, 5)(format, 0)); err != nil {
			return err
		}
		if err := winHRESULT("IDWriteFactory.CreateTextLayout", api.createLayout.bind(factory, 18)(factory,
			&text[0], uint32(len(text)), format, float32(width), math.MaxFloat32, &layout)); err != nil {
			return err
		}
		var count uint32
		// 查询所需大小的调用同时返回 E_NOT_SUFFICIENT_BUFFER 和所需数量。
		hr := api.lineMetrics.bind(layout, 59)(layout, nil, 0, &count)
		if hr < 0 && uint32(hr) != 0x8007007a {
			layout.release()
			return winHRESULT("IDWriteTextLayout.GetLineMetrics", hr)
		}
		if count == 0 {
			layout.release()
			return fmt.Errorf("overlay: DirectWrite returned no line metrics")
		}
		lines := state.layoutLines
		if cap(lines) < int(count) {
			lines = make([]winLineMetrics, count)
		} else {
			lines = lines[:count]
		}
		if err := winHRESULT("IDWriteTextLayout.GetLineMetrics", api.lineMetrics.bind(layout, 59)(layout, &lines[0], count, &count)); err != nil {
			layout.release()
			return err
		}
		if state.textLayout != nil {
			state.textLayout.release()
		}
		state.textLayout, state.layoutLines = layout, lines[:count]
		state.layoutSource, state.layoutFont = state.cfg.Text, state.cfg.Font
		state.layoutFontHeight, state.layoutWidth = state.fontHeight, width
		state.coverageValid = false
	}
	layout := state.textLayout
	// 累加实际视觉行的高度，不能假定换行符数量
	// 或 UTF-16 子串边界能够描述自动折行后的行。
	first, visibleHeight := len(state.layoutLines), float32(0)
	for first > 0 {
		height := state.layoutLines[first-1].Height
		if visibleHeight+height > float32(rect.Bottom-rect.Top) {
			break
		}
		visibleHeight += height
		first--
	}
	if visibleHeight == 0 {
		clear(state.coverage)
		state.coverageValid = false
		return nil
	}
	var hiddenHeight float32
	for _, line := range state.layoutLines[:first] {
		hiddenHeight += line.Height
	}
	if state.drawFactory == nil {
		result, _, _ := winD2DCreateFactory.Call(0, uintptr(unsafe.Pointer(&winD2DFactoryIID)), 0, uintptr(unsafe.Pointer(&state.drawFactory)))
		if err := winHRESULT("D2D1CreateFactory", int32(result)); err != nil {
			return err
		}
	}
	if state.drawTarget == nil {
		properties := winD2DProperties{Type: 1, Format: 87, AlphaMode: 3, DPIX: 96, DPIY: 96}
		if err := winHRESULT("ID2D1Factory.CreateDCRenderTarget",
			api.createTarget.bind(state.drawFactory, 16)(state.drawFactory, &properties, &state.drawTarget)); err != nil {
			return err
		}
	}
	target := state.drawTarget
	whole := winRect{Right: state.size.Width, Bottom: state.size.Height}
	if err := winHRESULT("ID2D1DCRenderTarget.BindDC", api.bindDC.bind(target, 57)(target, state.dc, &whole)); err != nil {
		return err
	}
	if state.textBrush == nil {
		white := [4]float32{1, 1, 1, 1}
		if err := winHRESULT("ID2D1RenderTarget.CreateSolidColorBrush",
			api.createBrush.bind(target, 8)(target, &white, nil, &state.textBrush)); err != nil {
			return err
		}
	}
	fullRange := winTextRange{Length: uint32(len(text))}
	// 释放画刷前先移除旧效果，并在不带颜色的情况下绘制覆盖率。
	// 黑色字形必须与白色字形具有相同的覆盖率。
	if err := winHRESULT("IDWriteTextLayout.SetDrawingEffect",
		api.drawingEffect.bind(layout, 38)(layout, nil, fullRange)); err != nil {
		return err
	}
	if !state.coverageValid || state.coverageRect != rect || state.coverageSize != state.size {
		if err := state.paintTextLayout(rect, visibleHeight, hiddenHeight); err != nil {
			return err
		}
		if ok, _, err := winGDIFlush.Call(); ok == 0 {
			return winError("GdiFlush", err)
		}
		length := len(state.pixels) / 4
		if cap(state.coverage) < length {
			state.coverage = make([]byte, length)
		} else {
			state.coverage = state.coverage[:length]
		}
		for i := range state.coverage {
			state.coverage[i] = state.pixels[i*4]
		}
		state.outline = state.outline[:0]
		state.coverageValid, state.coverageRect, state.coverageSize = true, rect, state.size
	}
	if state.cfg.Outline && len(state.outline) == 0 {
		state.updateTextOutline(rect)
	}
	if state.brushColors != state.cfg.Colors {
		for rgb, brush := range state.colorBrushes {
			brush.release()
			delete(state.colorBrushes, rgb)
		}
		state.brushColors = state.cfg.Colors
	}
	if state.colorBrushes == nil {
		state.colorBrushes = make(map[uint32]*winCOMObject, 6)
	}
	// TextRuns 使用 UTF-8 字节偏移；DirectWrite 区间使用 UTF-16 码元。
	// 只遍历源文本一次，并保留 emoji 的代理对。
	byteOffset, unitOffset := 0, uint32(0)
	for _, run := range state.cfg.TextRuns() {
		for byteOffset < run.Start {
			r, n := utf8.DecodeRuneInString(state.cfg.Text[byteOffset:])
			byteOffset += n
			if r > 0xffff {
				unitOffset += 2
			} else {
				unitOffset++
			}
		}
		start := unitOffset
		for byteOffset < run.End {
			r, n := utf8.DecodeRuneInString(state.cfg.Text[byteOffset:])
			byteOffset += n
			if r > 0xffff {
				unitOffset += 2
			} else {
				unitOffset++
			}
		}
		brush := state.colorBrushes[run.RGB]
		if brush == nil {
			color := [4]float32{float32(run.RGB>>16&255) / 255, float32(run.RGB>>8&255) / 255, float32(run.RGB&255) / 255, 1}
			if err := winHRESULT("ID2D1RenderTarget.CreateSolidColorBrush",
				api.createBrush.bind(target, 8)(target, &color, nil, &brush)); err != nil {
				return err
			}
			state.colorBrushes[run.RGB] = brush
		}
		if err := winHRESULT("IDWriteTextLayout.SetDrawingEffect",
			api.drawingEffect.bind(layout, 38)(layout, brush, winTextRange{start, unitOffset - start})); err != nil {
			return err
		}
	}
	err := state.paintTextLayout(rect, visibleHeight, hiddenHeight)
	runtime.KeepAlive(text)
	return err
}

func (state *winState) paintTextLayout(rect winRect, visibleHeight, hiddenHeight float32) error {
	api, target := &state.textMethods, state.drawTarget
	api.antialias.bind(target, 34)(target, 2)
	transform := [6]float32{1, 0, 0, 1, 0, 0}
	api.transform.bind(target, 30)(target, &transform)
	api.beginDraw.bind(target, 48)(target)
	black := [4]float32{0, 0, 0, 1}
	api.clear.bind(target, 47)(target, &black)
	clip := [4]float32{float32(rect.Left), float32(rect.Top), float32(rect.Right), float32(rect.Top) + visibleHeight}
	api.pushClip.bind(target, 45)(target, &clip, 1)
	transform[4], transform[5] = float32(rect.Left), float32(rect.Top)-hiddenHeight
	api.transform.bind(target, 30)(target, &transform)
	api.drawLayout.bind(target, 28)(target, winPointF{}, state.textLayout, state.textBrush, 0)
	api.popClip.bind(target, 46)(target)
	return winHRESULT("ID2D1RenderTarget.EndDraw", api.endDraw.bind(target, 49)(target, nil, nil))
}
