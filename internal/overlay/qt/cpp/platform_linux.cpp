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
#include <QDBusConnection>
#include <QDBusConnectionInterface>
#include <QDBusMessage>
#include <QDBusPendingCallWatcher>
#include <QDBusPendingReply>
#include <QDBusReply>
#include <QDBusServiceWatcher>
#include <QEventLoop>
#include <QTimer>
#include <QUuid>
#include <map>
#include <memory>

namespace awqt {

namespace {
const QString service = QStringLiteral("org.arcana.World.ElegantOverlay");
const QString path = QStringLiteral("/org/arcana/World/ElegantOverlay");
bool gnomeBridge = false;
struct WindowState {
    QString token;
    QString connector;
    QPoint position;
    bool placed = false;
    bool locked = true;
    bool inFlight = false;
    bool dirty = false;
};
struct Session : QObject {
    QString owner;
    QString error;
    std::function<void(const QString &)> failure;
    std::map<QWidget *, WindowState> windows;
    int pending = 0;
    QEventLoop *waiting = nullptr;
    void fail(const QString &detail) {
        if (!error.isEmpty()) return;
        error = QStringLiteral("Elegant overlay: GNOME window bridge: ") + detail;
        if (waiting) waiting->quit();
        failure(error);
    }
};
std::unique_ptr<Session> session;

QDBusMessage message(const QString &method, const QVariantList &arguments = {})
{
    auto call = QDBusMessage::createMethodCall(session->owner, path, service, method);
    call.setArguments(arguments);
    return call;
}
void sendPlacement(WindowState &state);

void send(const QString &method, const QVariantList &arguments, const QString &token = {})
{
    auto *state = session.get();
    ++state->pending;
    auto *watcher = new QDBusPendingCallWatcher(
        QDBusConnection::sessionBus().asyncCall(message(method, arguments), 5000), state);
    QObject::connect(watcher, &QDBusPendingCallWatcher::finished, state,
        [state, token](QDBusPendingCallWatcher *done) {
            const QDBusPendingReply<> reply = *done;
            --state->pending;
            // 已删除窗口的待处理 Place 不再影响新窗口。
            bool relevant = token.isEmpty();
            for (auto &entry : state->windows) {
                if (entry.second.token != token) continue;
                relevant = true;
                entry.second.inFlight = false;
                if (!reply.isError() && entry.second.dirty)
                    sendPlacement(entry.second);
                break;
            }
            if (reply.isError() && relevant) state->fail(reply.error().message());
            if (!state->pending && state->waiting) state->waiting->quit();
            done->deleteLater();
        });
}

void sendPlacement(WindowState &state)
{
    if (!state.placed) return;
    if (state.inFlight) {
        state.dirty = true;
        return;
    }
    state.inFlight = true;
    state.dirty = false;
    send(QStringLiteral("Place"), {state.token, state.connector,
        state.position.x(), state.position.y(), state.locked}, state.token);
}
} // namespace

QString beginWindows(std::function<void(const QString &)> failure)
{
    if (!gnomeBridge) return {};
    session = std::make_unique<Session>();
    session->failure = std::move(failure);
    const auto bus = QDBusConnection::sessionBus();
    if (!bus.isConnected() || !bus.interface()) {
        session.reset();
        return QStringLiteral("Elegant overlay: GNOME requires a connected session D-Bus.");
    }
    const QDBusReply<QString> owner = bus.interface()->serviceOwner(service);
    if (!owner.isValid() || owner.value().isEmpty()) {
        session.reset();
        return QStringLiteral("Elegant overlay: enable the Arcana Elegant GNOME Shell extension (protocol 1).");
    }
    session->owner = owner.value();
    auto *watcher = new QDBusServiceWatcher(service, bus, QDBusServiceWatcher::WatchForOwnerChange, session.get());
    QObject::connect(watcher, &QDBusServiceWatcher::serviceOwnerChanged, session.get(),
        [](const QString &, const QString &, const QString &owner) {
            if (owner != session->owner) session->fail(QStringLiteral("extension disabled or service owner changed"));
        });
    QObject::connect(bus.interface(), &QDBusConnectionInterface::serviceUnregistered, session.get(),
        [](const QString &name) {
            if (name == session->owner) session->fail(QStringLiteral("extension disconnected"));
        });
    const auto reply = bus.call(message(QStringLiteral("Begin"), {quint32(1)}), QDBus::Block, 5000);
    if (reply.type() == QDBusMessage::ErrorMessage) {
        const QString error = QStringLiteral("Elegant overlay: GNOME Begin failed: ") + reply.errorMessage();
        session.reset();
        return error;
    }
    return {};
}

QString awaitWindows()
{
    if (!session) return {};
    if (session->pending && session->error.isEmpty()) {
        // 嵌套事件循环继续交付 Wayland 映射和绘制事件，不轮询窗口。
        QEventLoop loop;
        QTimer deadline;
        deadline.setSingleShot(true);
        QObject::connect(&deadline, &QTimer::timeout, &loop, [] {
            session->fail(QStringLiteral("initial placement timed out"));
        });
        session->waiting = &loop;
        deadline.start(5000);
        loop.exec(QEventLoop::ExcludeUserInputEvents);
        session->waiting = nullptr;
    }
    return session->error;
}

void endWindows()
{
    if (!session) return;
    // 控制器已销毁窗口；最后释放 Shell 的独占会话。
    QDBusConnection::sessionBus().call(message(QStringLiteral("End")), QDBus::Block, 5000);
    session.reset();
}

void removeWindow(QWidget *widget)
{
    if (!session) return;
    const auto found = session->windows.find(widget);
    if (found == session->windows.end()) return;
    const QString token = found->second.token;
    session->windows.erase(found);
    send(QStringLiteral("Remove"), {token});
}
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
    if (gnomeBridge) {
        WindowState state;
        state.token = QUuid::createUuid().toString(QUuid::WithoutBraces);
        state.connector = screen->name();
        widget->setWindowTitle(QStringLiteral("Arcana Elegant ") + state.token);
        session->windows.emplace(widget, std::move(state));
        return;
    }
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
    if (gnomeBridge) {
        // 尺寸由 Qt 自行提交；Shell 只接收位置，最终位置服从 Mutter 约束。
        widget->resize(frame.size());
        auto &state = session->windows.at(widget);
        if (state.placed && state.position == frame.topLeft() && state.connector == screen->name()) return;
        state.placed = true;
        state.position = frame.topLeft();
        state.connector = screen->name();
        sendPlacement(state);
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
    if (gnomeBridge && session) {
        auto &state = session->windows.at(widget);
        if (state.locked != locked) {
            state.locked = locked;
            sendPlacement(state);
        }
    }
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
    if (!supported) {
        const auto desktops = qEnvironmentVariable("XDG_CURRENT_DESKTOP").split(QLatin1Char(':'));
        bool gnome = false;
        for (const auto &desktop : desktops)
            gnome = gnome || desktop.compare(QStringLiteral("GNOME"), Qt::CaseInsensitive) == 0;
        if (!gnome)
            return QStringLiteral("Elegant overlay: this Wayland compositor lacks layer-shell; ordinary-window fallback is disabled. "
                                  "GNOME requires the Arcana Elegant Shell extension.");
        gnomeBridge = true;
    }
    return {};
}
}
