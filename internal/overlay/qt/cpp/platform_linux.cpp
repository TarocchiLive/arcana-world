#include "platform.h"

#include <LayerShellQt/Window>
#include <QGuiApplication>
#include <QMargins>
#include <QScreen>
#include <QWidget>
#include <QWindow>
#include <QtGui/qguiapplication_platform.h>
#include <wayland-client.h>
#include <cstring>

namespace awqt {

// Linux 保留原有窗口角色；保存的任务栏偏好不改变 layer-shell 表面。
QString setTaskbarVisibility(bool) { return {}; }

static bool isWayland()
{
    return QGuiApplication::platformName().startsWith(QStringLiteral("wayland"));
}

void prepareWindow(QWidget *widget, QScreen *screen)
{
    auto *window = widget->windowHandle();
    window->setScreen(screen);
    if (!isWayland()) return;
    auto *layer = LayerShellQt::Window::get(window);
    layer->setScope(QStringLiteral("arcana-world-overlay"));
    layer->setLayer(LayerShellQt::Window::LayerOverlay);
    layer->setAnchors(QFlags<LayerShellQt::Window::Anchor>(LayerShellQt::Window::AnchorTop) | LayerShellQt::Window::AnchorLeft);
    // -1 不预留桌面区域，同时忽略面板保留区，以完整输出为定位基准。
    layer->setExclusiveZone(-1);
    layer->setKeyboardInteractivity(LayerShellQt::Window::KeyboardInteractivityNone);
    // 默认 ScreenFromQWindow：显式 QWindow::screen 决定 wl_output。
}

void placeWindow(QWidget *widget, QScreen *screen, const QRect &frame)
{
    if (!isWayland()) {
        widget->setGeometry(frame);
        return;
    }
    // layer-shell 没有普通窗口移动请求；锚点与输出内边距决定位置。
    const QPoint offset = frame.topLeft() - screen->geometry().topLeft();
    LayerShellQt::Window::get(widget->windowHandle())->setMargins(QMargins(offset.x(), offset.y(), 0, 0));
    widget->resize(frame.size());
    widget->update();
}

QString configureWindow(QWidget *widget, bool locked, bool)
{
    // 原生输入区域实现跨进程穿透，不用仅影响控件事件的透明属性冒充。
    if (auto *window = widget->windowHandle())
        window->setFlag(Qt::WindowTransparentForInput, locked);
    return {};
}

QString screenId(QScreen *screen) { return screen->name(); }
quint32 nativeDisplayId(QScreen *) { return 0; }

QString platformLimitation()
{
    if (!isWayland()) return {};
    auto *native = qGuiApp->nativeInterface<QNativeInterface::QWaylandApplication>();
    if (!native || !native->display())
        return QStringLiteral("Elegant overlay: cannot access the Wayland connection; install the bundled Wayland platform dependencies.");
    // 在创建任何浮层表面前检查实际连接，绝不回退成普通 Wayland 窗口。
    bool supported = false;
    const wl_registry_listener listener = {
        [](void *data, wl_registry *, uint32_t, const char *interface, uint32_t) {
            if (std::strcmp(interface, "zwlr_layer_shell_v1") == 0)
                *static_cast<bool *>(data) = true;
        },
        [](void *, wl_registry *, uint32_t) {}
    };
    wl_registry *registry = wl_display_get_registry(native->display());
    if (!registry)
        return QStringLiteral("Elegant overlay: cannot inspect the Wayland compositor registry.");
    wl_registry_add_listener(registry, &listener, &supported);
    const int status = wl_display_roundtrip(native->display());
    wl_registry_destroy(registry);
    if (status < 0)
        return QStringLiteral("Elegant overlay: lost the Wayland compositor connection while checking layer-shell support.");
    if (!supported)
        return QStringLiteral("Elegant overlay: this Wayland compositor does not support zwlr_layer_shell_v1. "
                              "Use a layer-shell-capable compositor such as KDE Plasma, Sway or Hyprland, "
                              "or log into an X11 session. Ordinary-window fallback is disabled.");
    return {};
}
}
