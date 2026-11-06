#pragma once

#include <QString>
#include <QtGlobal>
#include <QRect>
#include <QWidget>
#include <QWindow>
#include <functional>

class QWidget;
class QScreen;

namespace awqt {
#if defined(Q_OS_MACOS)
QString prepareApplication();
#endif
QString setTaskbarVisibility(bool visible);
#if defined(Q_OS_LINUX)
void prepareWindow(QWidget *widget, QScreen *screen);
void placeWindow(QWidget *widget, QScreen *screen, const QRect &frame);
QString beginWindows(std::function<void(const QString &)> failure);
QString awaitWindows();
void endWindows();
void removeWindow(QWidget *widget);
#else
inline void prepareWindow(QWidget *widget, QScreen *screen) { widget->windowHandle()->setScreen(screen); }
inline void placeWindow(QWidget *widget, QScreen *, const QRect &frame) { widget->setGeometry(frame); }
// 非 Linux 平台的原生窗口策略不需要会话握手。
inline QString beginWindows(std::function<void(const QString &)>) { return {}; }
inline QString awaitWindows() { return {}; }
inline void endWindows() {}
inline void removeWindow(QWidget *) {}
#endif
QString configureWindow(QWidget *widget, bool locked, bool showInTaskbar);
QString screenId(QScreen *screen);
#if defined(Q_OS_WIN)
QString screenIdForDisplay(quint32 index);
#else
quint32 nativeDisplayId(QScreen *screen);
#endif
QString platformLimitation();
}
