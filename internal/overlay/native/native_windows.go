//go:build windows && (386 || amd64 || arm64)

package native

import (
	"fmt"
	"syscall"

	"golang.org/x/sys/windows"
)

// 原生声明遵循 Win32 ABI；HWND 中不存储 Go 指针。
var (
	winUser              = windows.NewLazySystemDLL("user32.dll")
	winGDI               = windows.NewLazySystemDLL("gdi32.dll")
	winRegisterClass     = winUser.NewProc("RegisterClassExW")
	winUnregisterClass   = winUser.NewProc("UnregisterClassW")
	winCreateWindow      = winUser.NewProc("CreateWindowExW")
	winDestroyWindow     = winUser.NewProc("DestroyWindow")
	winDefWindowProc     = winUser.NewProc("DefWindowProcW")
	winEnumMonitors      = winUser.NewProc("EnumDisplayMonitors")
	winMonitorInfo       = winUser.NewProc("GetMonitorInfoW")
	winSetWindowPos      = winUser.NewProc("SetWindowPos")
	winMonitorFromWindow = winUser.NewProc("MonitorFromWindow")
	winPostMessage       = winUser.NewProc("PostMessageW")
	winShowWindow        = winUser.NewProc("ShowWindow")
	winUpdateLayered     = winUser.NewProc("UpdateLayeredWindow")
	winPeekMessage       = winUser.NewProc("PeekMessageW")
	winTranslateMessage  = winUser.NewProc("TranslateMessage")
	winDispatchMessage   = winUser.NewProc("DispatchMessageW")
	winWaitMessages      = winUser.NewProc("MsgWaitForMultipleObjectsEx")
	winSetDPIContext     = winUser.NewProc("SetThreadDpiAwarenessContext")
	winGetDPI            = winUser.NewProc("GetDpiForWindow")
	winCreateDC          = winGDI.NewProc("CreateCompatibleDC")
	winDeleteDC          = winGDI.NewProc("DeleteDC")
	winCreateDIB         = winGDI.NewProc("CreateDIBSection")
	winSelectObject      = winGDI.NewProc("SelectObject")
	winDeleteObject      = winGDI.NewProc("DeleteObject")
	winCreateFont        = winGDI.NewProc("CreateFontW")
	winGDIFlush          = winGDI.NewProc("GdiFlush")
	winWindowCallback    = syscall.NewCallback(winWindowProc)
	winMonitorCallback   = syscall.NewCallback(winCollectMonitor)
)

// 指针大小字段使用原生对齐，与 386、amd64 和 arm64 的 Win32 结构布局一致。
type winPoint struct{ X, Y int32 }
type winRect struct{ Left, Top, Right, Bottom int32 }
type winSize struct{ Width, Height int32 }
type winMessage struct {
	Window         uintptr
	Message        uint32
	WParam, LParam uintptr
	Time           uint32
	Point          winPoint
	Private        uint32
}
type winClass struct {
	Size, Style                                               uint32
	Procedure                                                 uintptr
	ClassExtra, WindowExtra                                   int32
	Instance, Icon, Cursor, Background, Menu, Name, SmallIcon uintptr
}
type winMonitorInfoEx struct {
	Size          uint32
	Monitor, Work winRect
	Flags         uint32
	Device        [32]uint16
}
type winBitmapInfo struct {
	Size                        uint32
	Width, Height               int32
	Planes, BitCount            uint16
	Compression, ImageSize      uint32
	XPels, YPels                int32
	ColorsUsed, ColorsImportant uint32
	Colors                      [1]uint32
}
type winBlend struct{ Operation, Flags, ConstantAlpha, AlphaFormat byte }

func winError(operation string, err error) error {
	if err == nil || err == syscall.Errno(0) {
		return fmt.Errorf("overlay: %s failed", operation)
	}
	return fmt.Errorf("overlay: %s: %w", operation, err)
}
