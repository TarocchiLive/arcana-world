#include "platform.h"

#include <QScreen>
#include <QWidget>
#include <QtGui/qscreen_platform.h>
#include <windows.h>

namespace awqt {
QString setTaskbarVisibility(bool) { return {}; }

QString configureWindow(QWidget *widget, bool locked, bool showInTaskbar)
{
    HWND window = reinterpret_cast<HWND>(widget->winId());
    SetLastError(ERROR_SUCCESS);
    const LONG_PTR previous = GetWindowLongPtrW(window, GWL_EXSTYLE);
    if (!previous && GetLastError() != ERROR_SUCCESS)
        return QStringLiteral("Elegant overlay: cannot read Windows extended window styles (%1).").arg(GetLastError());
    LONG_PTR style = previous | WS_EX_LAYERED | WS_EX_NOACTIVATE;
    // 分层窗口的透明扩展样式实现跨进程命中穿透，不只是忽略 Qt 鼠标事件。
    if (locked) style |= WS_EX_TRANSPARENT;
    else style &= ~WS_EX_TRANSPARENT;
    if (showInTaskbar) style = (style | WS_EX_APPWINDOW) & ~WS_EX_TOOLWINDOW;
    else style = (style | WS_EX_TOOLWINDOW) & ~WS_EX_APPWINDOW;
    const bool refreshShell = ((previous ^ style) & (WS_EX_APPWINDOW | WS_EX_TOOLWINDOW)) != 0;
    const bool wasVisible = IsWindowVisible(window);
    // Shell 要求先隐藏再改变任务栏样式；原生隐藏不销毁 Qt 内容，也不激活窗口。
    if (refreshShell && wasVisible) ShowWindow(window, SW_HIDE);
    if (style != previous) {
        SetLastError(ERROR_SUCCESS);
        if (!SetWindowLongPtrW(window, GWL_EXSTYLE, style) && GetLastError() != ERROR_SUCCESS) {
            const DWORD error = GetLastError();
            if (refreshShell && wasVisible) ShowWindow(window, SW_SHOWNOACTIVATE);
            return QStringLiteral("Elegant overlay: cannot set Windows extended window styles (%1).").arg(error);
        }
    }
    const BOOL positioned = SetWindowPos(window, HWND_TOPMOST, 0, 0, 0, 0,
        SWP_NOMOVE | SWP_NOSIZE | SWP_NOACTIVATE | SWP_FRAMECHANGED);
    const DWORD error = positioned ? ERROR_SUCCESS : GetLastError();
    if (refreshShell && wasVisible) ShowWindow(window, SW_SHOWNOACTIVATE);
    if (!positioned)
        return QStringLiteral("Elegant overlay: cannot apply Windows topmost window policy (%1).").arg(error);
    return {};
}

static QString monitorId(HMONITOR monitor)
{
    MONITORINFOEXW info{};
    info.cbSize = sizeof(info);
    if (!GetMonitorInfoW(monitor, &info)) return {};
    DISPLAY_DEVICEW device{};
    device.cb = sizeof(device);
    if (!EnumDisplayDevicesW(info.szDevice, 0, &device, EDD_GET_DEVICE_INTERFACE_NAME)) return {};
    return QString::fromWCharArray(device.DeviceID);
}

QString screenId(QScreen *screen)
{
    const auto *native = screen->nativeInterface<QNativeInterface::QWindowsScreen>();
    return native ? monitorId(native->handle()) : QString{};
}

struct MonitorIndex { quint32 target; quint32 index = 0; HMONITOR result = nullptr; };
static BOOL CALLBACK findMonitor(HMONITOR monitor, HDC, LPRECT, LPARAM data)
{
    auto &state = *reinterpret_cast<MonitorIndex *>(data);
    ++state.index;
    if (state.index == state.target) state.result = monitor;
    return TRUE;
}

QString screenIdForDisplay(quint32 index)
{
    MonitorIndex state{index};
    if (!EnumDisplayMonitors(nullptr, nullptr, findMonitor, reinterpret_cast<LPARAM>(&state)) || !state.result)
        return {};
    return monitorId(state.result);
}

QString platformLimitation() { return {}; }
}
