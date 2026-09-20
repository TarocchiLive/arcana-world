//go:build windows && (386 || amd64 || arm64)

package native

import (
	"arcana-world/internal/overlay"
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 在 Run 锁定的原生线程上同步枚举显示器。
type winMonitor struct {
	handle       uintptr
	bounds       winRect
	name         string
	id, friendly string
	primary      bool
}

var winEnumerated []winMonitor
var winEnumerationMutex sync.Mutex
var winEnumerationError error

func winCollectMonitor(handle, dc, rect, data uintptr) uintptr {
	info := winMonitorInfoEx{Size: uint32(unsafe.Sizeof(winMonitorInfoEx{}))}
	ok, _, err := winMonitorInfo.Call(handle, uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		winEnumerationError = winError("GetMonitorInfoW", err)
		return 0
	}
	device := winDisplayDevice{Size: uint32(unsafe.Sizeof(winDisplayDevice{}))}
	ok, _, err = winEnumDisplayDevices.Call(uintptr(unsafe.Pointer(&info.Device[0])), 0, uintptr(unsafe.Pointer(&device)), 1)
	if ok == 0 {
		winEnumerationError = winError("EnumDisplayDevicesW", err)
		return 0
	}
	id := windows.UTF16ToString(device.DeviceID[:])
	if id == "" {
		winEnumerationError = fmt.Errorf("overlay: Windows monitor has no device interface ID")
		return 0
	}
	winEnumerated = append(winEnumerated, winMonitor{
		handle: handle, bounds: info.Monitor, name: windows.UTF16ToString(info.Device[:]),
		id: id, friendly: windows.UTF16ToString(device.DeviceString[:]), primary: info.Flags&1 != 0,
	})
	return 1
}

func winMonitors() ([]winMonitor, error) {
	winEnumerationMutex.Lock()
	defer winEnumerationMutex.Unlock()
	winEnumerated = nil
	winEnumerationError = nil
	ok, _, err := winEnumMonitors.Call(0, 0, winMonitorCallback, 0)
	monitors := winEnumerated
	winEnumerated = nil
	if winEnumerationError != nil {
		return nil, winEnumerationError
	}
	if ok == 0 {
		return nil, winError("EnumDisplayMonitors", err)
	}
	return monitors, nil
}

func listDisplaysPlatform(ctx context.Context, cfg overlay.Config) ([]overlay.Display, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	monitors, err := winMonitors()
	if err != nil {
		return nil, err
	}
	names, err := winFriendlyNames(ctx)
	if err != nil {
		return nil, err
	}
	selected, err := cfg.SelectedDisplays()
	if err != nil {
		return nil, err
	}
	if selected == nil && len(monitors) > 0 {
		monitor, err := winSelectMonitor(cfg, monitors)
		if err != nil {
			return nil, err
		}
		selected = []string{monitor.id}
	}
	displays := make([]overlay.Display, 0, len(monitors))
	for _, monitor := range monitors {
		if name := names[strings.ToLower(monitor.name)]; name != "" {
			monitor.friendly = name
		}
		chosen := false
		for _, id := range selected {
			chosen = chosen || strings.EqualFold(id, monitor.id)
		}
		displays = append(displays, overlay.Display{ID: monitor.id, Name: monitor.friendly, Selected: chosen})
	}
	return displays, ctx.Err()
}

func winFriendlyNames(ctx context.Context) (map[string]string, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var pathCount, modeCount uint32
		result, _, _ := winDisplayBufferSizes.Call(2, uintptr(unsafe.Pointer(&pathCount)), uintptr(unsafe.Pointer(&modeCount)))
		if result != 0 {
			return nil, winError("GetDisplayConfigBufferSizes", syscall.Errno(result))
		}
		if pathCount == 0 {
			return nil, nil
		}
		paths := make([]winDisplayPath, pathCount)
		modes := make([]winDisplayMode, max(modeCount, 1))
		result, _, _ = winQueryDisplayConfig.Call(2, uintptr(unsafe.Pointer(&pathCount)), uintptr(unsafe.Pointer(&paths[0])),
			uintptr(unsafe.Pointer(&modeCount)), uintptr(unsafe.Pointer(&modes[0])), 0)
		if result == 122 { // ERROR_INSUFFICIENT_BUFFER: topology changed between calls.
			continue
		}
		if result != 0 {
			return nil, winError("QueryDisplayConfig", syscall.Errno(result))
		}
		names := make(map[string]string, pathCount)
		for _, path := range paths[:pathCount] {
			source := winDisplaySourceName{Header: winDisplayHeader{
				Type: 1, Size: uint32(unsafe.Sizeof(winDisplaySourceName{})), Adapter: path.SourceAdapter, ID: path.SourceID,
			}}
			target := winDisplayTargetName{Header: winDisplayHeader{
				Type: 2, Size: uint32(unsafe.Sizeof(winDisplayTargetName{})), Adapter: path.TargetAdapter, ID: path.TargetID,
			}}
			result, _, _ = winDisplayDeviceInfo.Call(uintptr(unsafe.Pointer(&source)))
			if result != 0 {
				return nil, winError("DisplayConfigGetDeviceInfo (source)", syscall.Errno(result))
			}
			result, _, _ = winDisplayDeviceInfo.Call(uintptr(unsafe.Pointer(&target)))
			if result != 0 {
				return nil, winError("DisplayConfigGetDeviceInfo (target)", syscall.Errno(result))
			}
			names[strings.ToLower(windows.UTF16ToString(source.Name[:]))] = windows.UTF16ToString(target.Friendly[:])
		}
		return names, nil
	}
}

// 调用者必须先确认枚举结果至少包含一台显示器。
func winPrimary(monitors []winMonitor) winMonitor {
	for _, monitor := range monitors {
		if monitor.primary {
			return monitor
		}
	}
	return monitors[0]
}

func winSelectMonitor(cfg overlay.Config, monitors []winMonitor) (winMonitor, error) {
	if len(monitors) == 0 {
		return winMonitor{}, fmt.Errorf("overlay: no Windows desktop monitors available")
	}
	if cfg.Output != "" {
		for _, monitor := range monitors {
			if strings.EqualFold(monitor.name, cfg.Output) {
				return monitor, nil
			}
		}
		return winMonitor{}, fmt.Errorf("overlay: Windows output %q not found", cfg.Output)
	}
	if cfg.DisplayID != 0 {
		// 枚举编号只在选择时解析；后续热插拔按稳定设备 ID 恢复。
		if uint64(cfg.DisplayID) > uint64(len(monitors)) {
			return winMonitor{}, fmt.Errorf("overlay: Windows display ID %d not found", cfg.DisplayID)
		}
		return monitors[cfg.DisplayID-1], nil
	}
	return winPrimary(monitors), nil
}

func (state *winState) refreshGeometry() error {
	state.geometryDirty = false
	monitor := state.target
	if monitor.handle == 0 {
		winShowWindow.Call(state.window, 0)
		state.monitor = 0
		return nil
	}
	currentMonitor, _, _ := winMonitorFromWindow.Call(state.window, 0)
	if state.monitor != monitor.handle || currentMonitor != monitor.handle {
		// 在查询 GetDpiForWindow 前移动窗口但不激活它：与 DC 的
		// LOGPIXELSX 不同，这是实际目标显示器的按显示器感知 DPI。
		ok, _, err := winSetWindowPos.Call(state.window, 0, uintptr(monitor.bounds.Left), uintptr(monitor.bounds.Top), 0, 0, 0x0015)
		if ok == 0 {
			return winError("SetWindowPos (monitor)", err)
		}
		state.monitor = monitor.handle
	}
	dpi, _, err := winGetDPI.Call(state.window)
	if dpi == 0 {
		return winError("GetDpiForWindow", err)
	}
	scale := float64(dpi) / 96
	bounds := monitor.bounds
	availableWidth := int64(bounds.Right) - int64(bounds.Left)
	availableHeight := int64(bounds.Bottom) - int64(bounds.Top)
	if availableWidth <= 0 || availableHeight <= 0 {
		return fmt.Errorf("overlay: invalid Windows monitor bounds")
	}
	// 在转换前以浮点数进行钳制：即使逻辑设置为有限值，
	// 与 DPI 相乘后仍可能溢出。边界差值使用 int64，因为
	// 负的虚拟桌面原点可能使范围超过有符号 32 位整数。
	frame := state.cfg.Frame(overlay.Rect{Width: float64(availableWidth) / scale, Height: float64(availableHeight) / scale})
	width := int64(math.Max(1, math.Min(math.Round(frame.Width*scale), float64(availableWidth))))
	height := int64(math.Max(1, math.Min(math.Round(frame.Height*scale), float64(availableHeight))))
	// 同时限制原生有符号尺寸以及 Go/原生 DIB 分配大小。
	if width > 32767 || height > 32767 || width*height > 16*1024*1024 {
		return fmt.Errorf("overlay: Windows panel exceeds the 64 MiB rendering limit")
	}
	fontPixels := math.Round(state.cfg.Font.Size * scale)
	if math.IsInf(fontPixels, 0) || fontPixels > 32767 {
		return fmt.Errorf("overlay: Windows font size exceeds 32767 pixels")
	}
	fontHeight := int32(math.Max(1, fontPixels))
	x := int64(bounds.Left) + int64(math.Max(0, math.Min(math.Round(frame.X*scale), float64(availableWidth-width))))
	y := int64(bounds.Top) + int64(math.Max(0, math.Min(math.Round(frame.Y*scale), float64(availableHeight-height))))
	if x < math.MinInt32 || x > math.MaxInt32 || y < math.MinInt32 || y > math.MaxInt32 {
		return fmt.Errorf("overlay: Windows panel position out of range")
	}
	state.position = winPoint{int32(x), int32(y)}
	content := state.cfg.ContentRect(float64(width)/scale, float64(height)/scale)
	state.textRect = winRect{
		int32(math.Round(content.X * scale)), int32(math.Round(content.Y * scale)),
		int32(math.Min(float64(width), math.Round((content.X+content.Width)*scale))),
		int32(math.Min(float64(height), math.Round((content.Y+content.Height)*scale))),
	}
	return state.prepareSurface(winSize{int32(width), int32(height)}, fontHeight)
}
