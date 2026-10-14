#pragma once

#include <QString>
#include <QtGlobal>
#include <QRect>
#include <QWidget>
#include <QWindow>

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
#else
inline void prepareWindow(QWidget *widget, QScreen *screen) { widget->windowHandle()->setScreen(screen); }
inline void placeWindow(QWidget *widget, QScreen *, const QRect &frame) { widget->setGeometry(frame); }
#endif
QString configureWindow(QWidget *widget, bool locked, bool showInTaskbar);
QString screenId(QScreen *screen);
quint32 nativeDisplayId(QScreen *screen);
QString platformLimitation();
}
