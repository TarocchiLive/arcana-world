//go:build windows && (386 || amd64 || arm64)

package native

import (
	"fmt"
	"math"
	"runtime"
	"syscall"
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

type winTextLayoutMetrics struct {
	Left, Top, Width, WidthWithTrailingWhitespace, Height float32
	LayoutWidth, LayoutHeight                             float32
	MaxBidiDepth, LineCount                               uint32
}

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
	layoutMetrics winMethod[func(*winCOMObject, *winTextLayoutMetrics) int32]
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
	var metrics winTextMetrics
	if ok, _, err := winGetTextMetrics.Call(state.dc, uintptr(unsafe.Pointer(&metrics))); ok == 0 {
		return winError("GetTextMetricsW", err)
	}
	if metrics.Height <= 0 || metrics.Ascent <= 0 {
		return fmt.Errorf("overlay: Windows font returned invalid line metrics")
	}
	capacity := (rect.Bottom - rect.Top) / metrics.Height
	if capacity == 0 {
		return nil
	}
	family := state.cfg.Font.Family
	if family == "" {
		family = "Segoe UI"
	}
	face, err := windows.UTF16FromString(family)
	if err != nil {
		return fmt.Errorf("overlay: Windows font family: %w", err)
	}
	locale := [1]uint16{0}
	var format, layout *winCOMObject
	api := &state.textMethods
	if state.writeFactory == nil {
		hr, _, _ := winDWriteCreateFactory.Call(0, uintptr(unsafe.Pointer(&winDWriteFactoryIID)), uintptr(unsafe.Pointer(&state.writeFactory)))
		if err := winHRESULT("DWriteCreateFactory", int32(hr)); err != nil {
			return err
		}
	}
	factory := state.writeFactory
	var style uint32
	if state.cfg.Font.Italic {
		style = 2 // DWRITE_FONT_STYLE_ITALIC
	}
	hr := api.createFormat.bind(factory, 15)(factory, &face[0], nil,
		uint32(state.cfg.Font.Weight), style, 5, float32(state.fontHeight), &locale[0], &format)
	if err := winHRESULT("IDWriteFactory.CreateTextFormat", hr); err != nil {
		return err
	}
	defer format.release()
	if err := winHRESULT("IDWriteTextFormat.SetLineSpacing",
		api.lineSpacing.bind(format, 10)(format, 1, float32(metrics.Height), float32(metrics.Ascent))); err != nil {
		return err
	}
	// 使用原生词边界；超长词仍可在字形簇边界换行。
	if err := winHRESULT("IDWriteTextFormat.SetWordWrapping", api.wordWrapping.bind(format, 5)(format, 0)); err != nil {
		return err
	}
	hr = api.createLayout.bind(factory, 18)(factory, &text[0], uint32(len(text)), format,
		float32(rect.Right-rect.Left), math.MaxFloat32, &layout)
	if err := winHRESULT("IDWriteFactory.CreateTextLayout", hr); err != nil {
		return err
	}
	defer layout.release()
	var bounds winTextLayoutMetrics
	if err := winHRESULT("IDWriteTextLayout.GetMetrics", api.layoutMetrics.bind(layout, 60)(layout, &bounds)); err != nil {
		return err
	}
	visible := min(uint32(capacity), bounds.LineCount)
	if visible == 0 {
		return nil
	}
	// 工厂和软件渲染目标随窗口复用；BindDC 使其跟随 DIB 替换。
	if state.drawFactory == nil {
		result, _, _ := winD2DCreateFactory.Call(0, uintptr(unsafe.Pointer(&winD2DFactoryIID)), 0, uintptr(unsafe.Pointer(&state.drawFactory)))
		if err := winHRESULT("D2D1CreateFactory", int32(result)); err != nil {
			return err
		}
	}
	// 目标设为 96 DPI，使其逻辑像素与已缩放的 GDI 物理像素一一对应。
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
	api.antialias.bind(target, 34)(target, 2) // 灰度抗锯齿，避免 ClearType 彩边。
	transform := [6]float32{1, 0, 0, 1, 0, 0}
	api.transform.bind(target, 30)(target, &transform)
	api.beginDraw.bind(target, 48)(target)
	black := [4]float32{0, 0, 0, 1}
	api.clear.bind(target, 47)(target, &black)
	clip := [4]float32{float32(rect.Left), float32(rect.Top), float32(rect.Right),
		float32(rect.Top) + float32(visible)*float32(metrics.Height)}
	api.pushClip.bind(target, 45)(target, &clip, 1)
	// 在单位变换下固定裁剪区，再一次平移整段布局，保持绘制开销与字形数量线性相关。
	transform[4] = float32(rect.Left)
	transform[5] = float32(rect.Top) - float32(bounds.LineCount-visible)*float32(metrics.Height)
	api.transform.bind(target, 30)(target, &transform)
	api.drawLayout.bind(target, 28)(target, winPointF{}, layout, state.textBrush, 0)
	api.popClip.bind(target, 46)(target)
	err = winHRESULT("ID2D1RenderTarget.EndDraw", api.endDraw.bind(target, 49)(target, nil, nil))
	// 保留原生布局引用的输入，直到本次绘制结束。
	runtime.KeepAlive(text)
	return err
}
