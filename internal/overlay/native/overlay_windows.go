//go:build windows && (386 || amd64 || arm64)

package native

import (
	"arcana-world/internal/overlay"
	"context"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// 这是桌面合成器叠加层，而非注入式游戏叠加层。无法保证独占
// 全屏或安全桌面上的可见性；普通置顶窗口可能遮挡它。它不会激活窗口，
// 也不会获取输入。
type winState struct {
	window                                           uintptr
	cfg                                              overlay.Config
	target                                           winMonitor
	monitor                                          uintptr
	dirty, geometryDirty                             bool
	dc, bitmap, font, originalBitmap, originalFont   uintptr
	writeFactory, drawFactory, drawTarget, textBrush *winCOMObject
	textMethods                                      winTextMethods
	textLayout                                       *winCOMObject
	layoutSource                                     string
	layoutFont                                       overlay.Font
	layoutFontHeight, layoutWidth                    int32
	layoutLines                                      []winLineMetrics
	coverage                                         []byte
	outline, outlineScratch                          []byte
	outlineRadius                                    int
	coverageValid                                    bool
	coverageRect                                     winRect
	coverageSize                                     winSize
	colorBrushes                                     map[uint32]*winCOMObject
	brushColors                                      overlay.Colors
	pixels                                           []byte
	size                                             winSize
	position                                         winPoint
	textRect                                         winRect
	fontHeight                                       int32
	fontConfig                                       overlay.Font
	encodedSource                                    string
	textBuffer                                       []uint16
	painted                                          bool
	paintedConfig                                    overlay.Config
	paintedSize                                      winSize
	paintedRect                                      winRect
	paintedFontHeight                                int32
}

// 只有 Run 锁定的原生线程会访问此回调目标。回调函数
// 在 native_windows.go 中仅分配一次，而不是每个窗口分配一次。
var winActive *winSession

type winSession struct {
	cfg                          overlay.Config
	windows                      map[string]*winState
	className                    []uint16
	instance                     uintptr
	legacy                       string
	dirty, geometryDirty, closed bool
	ready                        func()
}

func (session *winSession) close() {
	for id, state := range session.windows {
		delete(session.windows, id)
		winDestroyWindow.Call(state.window)
		state.closeResources()
	}
}

func (session *winSession) render() error {
	session.dirty = false
	if session.geometryDirty {
		session.geometryDirty = false
		selected, err := session.cfg.SelectedDisplays()
		if err != nil {
			return err
		}
		monitors, err := winMonitors()
		if err != nil {
			return err
		}
		wanted := make(map[string]winMonitor)
		if selected == nil {
			if session.legacy == "" {
				monitor, err := winSelectMonitor(session.cfg, monitors)
				if err != nil {
					return err
				}
				session.legacy = monitor.id
			}
			selected = []string{session.legacy}
			// 旧单屏模式临时回退，保留原始身份以便重连后恢复。
			if len(monitors) > 0 {
				found := false
				for _, monitor := range monitors {
					found = found || strings.EqualFold(monitor.id, session.legacy)
				}
				if !found {
					selected = []string{winPrimary(monitors).id}
				}
			}
		}
		for _, monitor := range monitors {
			for _, id := range selected {
				if strings.EqualFold(monitor.id, id) {
					wanted[monitor.id] = monitor
					break
				}
			}
		}
		for id, state := range session.windows {
			if _, keep := wanted[id]; !keep {
				delete(session.windows, id)
				winDestroyWindow.Call(state.window)
				state.closeResources()
			}
		}
		for id, monitor := range wanted {
			state := session.windows[id]
			if state == nil {
				window, _, err := winCreateWindow.Call(0x080800A8, uintptr(unsafe.Pointer(&session.className[0])), 0, 0x80000000,
					uintptr(monitor.bounds.Left), uintptr(monitor.bounds.Top), 1, 1, 0, 0, session.instance, 0)
				runtime.KeepAlive(session.className)
				if window == 0 {
					return winError("CreateWindowExW", err)
				}
				state = &winState{window: window}
				session.windows[id] = state
			}
			state.target, state.geometryDirty, state.dirty = monitor, true, true
		}
	}
	for _, state := range session.windows {
		if state.cfg != session.cfg {
			next, prior := session.cfg, state.cfg
			if next.Position != prior.Position || next.Width != prior.Width || next.Height != prior.Height || next.Padding != prior.Padding || next.Font != prior.Font {
				state.geometryDirty = true
			}
			state.cfg, state.dirty = next, true
		}
		if state.dirty {
			if err := state.render(); err != nil {
				return err
			}
		}
	}
	if session.ready != nil {
		session.ready()
		session.ready = nil
	}
	return nil
}

func winWindowProc(window uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case 0x0084: // WM_NCHITTEST；分层窗口的 WS_EX_TRANSPARENT 处理跨线程穿透。
		return ^uintptr(0) // HTTRANSPARENT
	case 0x0021: // WM_MOUSEACTIVATE
		return 3 // MA_NOACTIVATE
	case 0x007E, 0x02E0, 0x0219: // WM_DISPLAYCHANGE, WM_DPICHANGED, WM_DEVICECHANGE
		if winActive != nil {
			winActive.dirty, winActive.geometryDirty = true, true
		}
		return 0
	case 0x0010: // WM_CLOSE
		if winActive != nil {
			winActive.closed = true
		}
		return 0
	}
	result, _, _ := winDefWindowProc.Call(window, uintptr(message), wParam, lParam)
	return result
}

func runPlatform(ctx context.Context, cfg overlay.Config, updates <-chan overlay.Config, ready func()) error {
	// 集成的 Unix 域套接字要求 Windows 10 1803；线程 DPI 设置不影响宿主窗口。
	version := windows.RtlGetVersion()
	if version.MajorVersion < 10 || version.MajorVersion == 10 && version.BuildNumber < 17134 {
		return fmt.Errorf("overlay: Windows 10 1803 or newer is required")
	}
	if err := winSetDPIContext.Find(); err != nil {
		return fmt.Errorf("overlay: SetThreadDpiAwarenessContext unavailable: %w", err)
	}
	if err := winGetDPI.Find(); err != nil {
		return fmt.Errorf("overlay: GetDpiForWindow unavailable: %w", err)
	}
	previous, _, err := winSetDPIContext.Call(^uintptr(3)) // DPI_AWARENESS_CONTEXT_PER_MONITOR_AWARE_V2 (-4)
	if previous == 0 {
		return winError("SetThreadDpiAwarenessContext", err)
	}
	defer winSetDPIContext.Call(previous)
	if err := ctx.Err(); err != nil {
		return err
	}
	state := &winSession{cfg: cfg, windows: make(map[string]*winState), ready: ready, dirty: true, geometryDirty: true}
	className, _ := windows.UTF16FromString("ArcanaWorldNativeOverlay")
	var instance windows.Handle
	if err := windows.GetModuleHandleEx(windows.GET_MODULE_HANDLE_EX_FLAG_UNCHANGED_REFCOUNT, nil, &instance); err != nil {
		return fmt.Errorf("overlay: GetModuleHandleEx: %w", err)
	}
	class := winClass{Size: uint32(unsafe.Sizeof(winClass{})), Procedure: winWindowCallback, Instance: uintptr(instance), Name: uintptr(unsafe.Pointer(&className[0]))}
	atom, _, err := winRegisterClass.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return winError("RegisterClassExW", err)
	}
	defer func() {
		winUnregisterClass.Call(uintptr(unsafe.Pointer(&className[0])), uintptr(instance))
		runtime.KeepAlive(className)
	}()
	winActive = state
	defer func() { winActive = nil }()
	state.className, state.instance = className, uintptr(instance)
	defer state.close()
	// WS_EX_LAYERED | TRANSPARENT | NOACTIVATE | TOOLWINDOW | TOPMOST;
	// WS_POPUP 不带 WS_VISIBLE 可使创建过程完全不激活窗口。
	window, _, err := winCreateWindow.Call(0x080800A8, uintptr(unsafe.Pointer(&className[0])), 0, 0x80000000,
		0, 0, 1, 1, 0, 0, uintptr(instance), 0)
	runtime.KeepAlive(className)
	if window == 0 {
		return winError("CreateWindowExW", err)
	}
	defer winDestroyWindow.Call(window)

	// 一个自动重置事件承载合并后的配置和取消信号。没有 Go 指针
	// 跨越消息队列；生产者的生命周期不得超过其事件或 HWND 的生命周期。
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return fmt.Errorf("overlay: CreateEvent: %w", err)
	}
	defer windows.CloseHandle(event)
	if err := state.render(); err != nil {
		return err
	}
	var mailbox struct {
		sync.Mutex
		cfg     overlay.Config
		changed bool
		err     error
	}
	stop, joined := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(joined)
		last := cfg
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				// 取消操作需要与更新相同的独立唤醒路径；若信号发送失败，
				// 原生等待可能一直挂起。
				if err := windows.SetEvent(event); err != nil {
					winPostMessage.Call(window, 0, 0, 0)
				}
				return
			case cfg, open := <-updates:
				if !open {
					updates = nil
					continue
				}
				cfg, normalizeErr := cfg.Normalize()
				if normalizeErr == nil && cfg == last {
					continue
				}
				last = cfg
				mailbox.Lock()
				if normalizeErr != nil {
					mailbox.err = normalizeErr
				} else {
					mailbox.cfg, mailbox.changed = cfg, true
				}
				mailbox.Unlock()
				if err := windows.SetEvent(event); err != nil {
					mailbox.Lock()
					mailbox.err = fmt.Errorf("overlay: SetEvent: %w", err)
					mailbox.Unlock()
					// 有效且仍存活的事件通常不会失败。WM_NULL 是一条独立的唤醒路径，
					// 因此原生循环可以报告失败。
					winPostMessage.Call(window, 0, 0, 0)
					return
				}
				if normalizeErr != nil {
					return
				}
			}
		}
	}()
	// 在关闭事件或销毁窗口之前等待生产者结束。尤其要确保
	// 已进入 SetEvent/PostMessage 的生产者仍持有有效句柄。
	defer func() { close(stop); <-joined }()
	for {
		if ctx.Err() != nil || state.closed {
			return nil
		}
		mailbox.Lock()
		next, changed, wakeErr := mailbox.cfg, mailbox.changed, mailbox.err
		mailbox.changed = false
		mailbox.Unlock()
		if wakeErr != nil {
			return wakeErr
		}
		if changed && next != state.cfg {
			if next.Output != state.cfg.Output || next.DisplayID != state.cfg.DisplayID || next.Displays != state.cfg.Displays {
				state.legacy = ""
				state.geometryDirty = true
			}
			state.cfg, state.dirty = next, true
		}
		// 限制每次排空的消息数量，避免持续繁忙的桌面使更新
		// 或取消操作无法获得调度。MWMO_INPUTAVAILABLE 处理剩余排队输入。
		for range 256 {
			var message winMessage
			ok, _, _ := winPeekMessage.Call(uintptr(unsafe.Pointer(&message)), 0, 0, 0, 1)
			if ok == 0 {
				break
			}
			if message.Message == 0x0012 {
				return nil
			} // WM_QUIT
			winTranslateMessage.Call(uintptr(unsafe.Pointer(&message)))
			winDispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
		}
		if ctx.Err() != nil || state.closed {
			return nil
		}
		if state.dirty {
			if err := state.render(); err != nil {
				return err
			}
		}
		if state.dirty {
			continue
		}
		result, _, err := winWaitMessages.Call(1, uintptr(unsafe.Pointer(&event)), 0xFFFFFFFF, 0x04FF, 0x0004)
		if result == 0xFFFFFFFF {
			return winError("MsgWaitForMultipleObjectsEx", err)
		}
		if result != 0 && result != 1 {
			return fmt.Errorf("overlay: unexpected message wait result %#x", result)
		}
	}
}
