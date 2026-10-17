#include "platform.h"

#include <QScreen>
#include <QWidget>
#include <QtGui/qscreen_platform.h>
#import <Cocoa/Cocoa.h>
#import <CoreGraphics/CoreGraphics.h>

// Qt 将已有代理保留为 reflectionDelegate；拒绝 AppKit 的空文档与重开激活路径。
@interface AWOverlayApplicationDelegate : NSObject <NSApplicationDelegate>
@end

@implementation AWOverlayApplicationDelegate
- (BOOL)applicationShouldOpenUntitledFile:(NSApplication *)application { return NO; }
- (BOOL)applicationOpenUntitledFile:(NSApplication *)application { return NO; }
- (BOOL)applicationShouldHandleReopen:(NSApplication *)application hasVisibleWindows:(BOOL)visible { return NO; }
@end

namespace awqt {

QString prepareApplication()
{
    NSApplication *app = NSApplication.sharedApplication;
    static AWOverlayApplicationDelegate *delegate = [[AWOverlayApplicationDelegate alloc] init];
    app.delegate = delegate;
    return setTaskbarVisibility(false);
}

QString setTaskbarVisibility(bool visible)
{
    NSApplication *app = NSApplication.sharedApplication;
    const auto policy = visible ? NSApplicationActivationPolicyRegular : NSApplicationActivationPolicyAccessory;
    if (app.activationPolicy != policy && ![app setActivationPolicy:policy])
        return QStringLiteral("Elegant overlay: macOS refused the requested Dock activation policy.");
    return {};
}

QString configureWindow(QWidget *widget, bool locked, bool)
{
    NSView *view = reinterpret_cast<NSView *>(widget->winId());
    NSWindow *window = view.window;
    if (!window) return QStringLiteral("Elegant overlay: cannot access the Cocoa overlay window.");
    window.opaque = NO;
    window.backgroundColor = NSColor.clearColor;
    window.hasShadow = NO;
    window.level = NSFloatingWindowLevel;
    window.hidesOnDeactivate = NO;
    window.ignoresMouseEvents = locked;
    window.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces
        | NSWindowCollectionBehaviorFullScreenAuxiliary
        | NSWindowCollectionBehaviorStationary
        | NSWindowCollectionBehaviorIgnoresCycle;
    // Qt::Tool 在 Cocoa 上创建 NSPanel，非激活面板避免点击和更新抢焦点。
    if ([window isKindOfClass:NSPanel.class]) {
        NSPanel *panel = static_cast<NSPanel *>(window);
        panel.styleMask |= NSWindowStyleMaskNonactivatingPanel;
        panel.becomesKeyOnlyIfNeeded = YES;
        panel.floatingPanel = YES;
    }
    return {};
}

quint32 nativeDisplayId(QScreen *screen)
{
    const auto *native = screen->nativeInterface<QNativeInterface::QCocoaScreen>();
    if (!native) return 0;
    return [native->nativeScreen().deviceDescription[@"NSScreenNumber"] unsignedIntValue];
}

QString screenId(QScreen *screen)
{
    const auto display = nativeDisplayId(screen);
    if (!display) return {};
    CFUUIDRef uuid = CGDisplayCreateUUIDFromDisplayID(display);
    if (!uuid) return {};
    CFStringRef text = CFUUIDCreateString(kCFAllocatorDefault, uuid);
    QString result = QString::fromCFString(text);
    CFRelease(text);
    CFRelease(uuid);
    return result;
}

QString platformLimitation() { return {}; }
}
