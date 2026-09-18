//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#import <CoreGraphics/CoreGraphics.h>
#import "panel_darwin.h"

// 所有窗口、字体和绘制状态只由主线程持有；生产者只写最新快照。
@interface AWOverlayState : NSObject
@property AWOverlayConfig config;
@property(nonatomic, copy) NSString *text;
@property(nonatomic, copy) NSString *family;
@end
@implementation AWOverlayState
@end

static AWOverlayState *copyState(AWOverlayConfig config) {
    AWOverlayState *state = [AWOverlayState new];
    state.text = [NSString stringWithUTF8String:config.text];
    state.family = [NSString stringWithUTF8String:config.family];
    config.text = NULL;
    config.family = NULL;
    state.config = config;
    return state;
}

@interface AWOverlayPanel : NSPanel
@end
@implementation AWOverlayPanel
- (BOOL)canBecomeKeyWindow { return NO; }
- (BOOL)canBecomeMainWindow { return NO; }
// 以完整显示器边界为准，不使用菜单栏或 Dock 的工作区。
- (NSRect)constrainFrameRect:(NSRect)frame toScreen:(NSScreen *)screen { return frame; }
@end

@interface AWOverlayText : NSView
@property(nonatomic, copy) NSString *text;
@property(nonatomic, strong) NSFont *font;
@property(nonatomic, strong) NSColor *color;
@end
@implementation AWOverlayText {
    NSTextStorage *_storage;
    NSLayoutManager *_layout;
    NSTextContainer *_container;
    NSString *_laidOutText;
    NSFont *_laidOutFont;
    NSColor *_laidOutColor;
    CGFloat _lineHeight;
    CGFloat _baseline;
}
- (BOOL)isFlipped { return YES; }
- (BOOL)acceptsFirstResponder { return NO; }
- (void)drawRect:(NSRect)dirtyRect {
    NSRect bounds = self.bounds;
    if (NSWidth(bounds) <= 0 || NSHeight(bounds) <= 0 || !self.text.length || !self.font) return;
    if (!_layout) {
        _storage = [NSTextStorage new];
        _layout = [NSLayoutManager new];
        _layout.usesFontLeading = YES;
        _container = [[NSTextContainer alloc] initWithContainerSize:NSMakeSize(NSWidth(bounds), CGFLOAT_MAX)];
        _container.lineFragmentPadding = 0;
        [_layout addTextContainer:_container];
        [_storage addLayoutManager:_layout];
    }
    if (![_laidOutText isEqualToString:self.text] || ![_laidOutFont isEqual:self.font] ||
        ![_laidOutColor isEqual:self.color]) {
        // 按配置字体的逻辑点取整行高，避免回退字体或 Retina 缩放改变行容量。
        _lineHeight = ceil([_layout defaultLineHeightForFont:self.font]);
        _baseline = self.font.ascender +
            MAX(0, (_lineHeight - self.font.ascender + self.font.descender) / 2);
        NSMutableParagraphStyle *paragraph = [NSMutableParagraphStyle new];
        paragraph.lineBreakMode = NSLineBreakByWordWrapping;
        paragraph.minimumLineHeight = _lineHeight;
        paragraph.maximumLineHeight = _lineHeight;
        [_storage setAttributedString:[[NSAttributedString alloc] initWithString:self.text attributes:@{
            NSFontAttributeName:self.font,
            NSForegroundColorAttributeName:self.color,
            NSParagraphStyleAttributeName:paragraph
        }]];
        _laidOutText = [self.text copy];
        _laidOutFont = self.font;
        _laidOutColor = self.color;
    }
    if (!(_lineHeight > 0) || NSHeight(bounds) < _lineHeight) return;
    if (_container.containerSize.width != NSWidth(bounds))
        _container.containerSize = NSMakeSize(NSWidth(bounds), CGFLOAT_MAX);
    [_layout ensureLayoutForTextContainer:_container];
    NSRange allGlyphs = [_layout glyphRangeForTextContainer:_container];
    __block NSUInteger lineCount = 0;
    [_layout enumerateLineFragmentsForGlyphRange:allGlyphs usingBlock:
        ^(NSRect rect, NSRect used, NSTextContainer *container, NSRange glyphs, BOOL *stop) {
            ++lineCount;
        }];
    // TextKit 的末尾空行没有 glyph，但仍是一个完整视觉行。
    if (_layout.extraLineFragmentTextContainer == _container) ++lineCount;
    CGFloat capacity = floor(NSHeight(bounds) / _lineHeight);
    NSUInteger visible = capacity >= (CGFloat)lineCount ? lineCount : (NSUInteger)capacity;
    NSUInteger first = lineCount - visible;
    __block NSUInteger index = 0;
    [NSGraphicsContext saveGraphicsState];
    NSRectClip(bounds);
    [_layout enumerateLineFragmentsForGlyphRange:allGlyphs usingBlock:
        ^(NSRect rect, NSRect used, NSTextContainer *container, NSRange glyphs, BOOL *stop) {
            NSUInteger row = index++;
            if (row < first || !glyphs.length) return;
            CGFloat top = NSMinY(bounds) + (row - first) * self->_lineHeight;
            // 保留整形结果并对齐基线；超高回退字形裁剪在本行槽内。
            NSPoint location = [self->_layout locationForGlyphAtIndex:glyphs.location];
            NSPoint origin = NSMakePoint(NSMinX(bounds),
                top + self->_baseline - NSMinY(rect) - location.y);
            [NSGraphicsContext saveGraphicsState];
            NSRectClip(NSMakeRect(NSMinX(bounds), top, NSWidth(bounds), self->_lineHeight));
            [self->_layout drawGlyphsForGlyphRange:glyphs atPoint:origin];
            [NSGraphicsContext restoreGraphicsState];
        }];
    [NSGraphicsContext restoreGraphicsState];
}
@end

static NSScreen *screenForID(unsigned int display) {
    if (!display) return NSScreen.mainScreen ?: NSScreen.screens.firstObject;
    for (NSScreen *screen in NSScreen.screens) {
        if ([screen.deviceDescription[@"NSScreenNumber"] unsignedIntValue] == display) return screen;
    }
    return nil;
}

static CGFloat axisOffset(int anchor, double offset, CGFloat available) {
    double value = anchor == 0 ? offset : anchor == 1 ? available / 2 + offset : available - offset;
    return MAX(0, MIN(available, value));
}

static NSFont *fontForState(AWOverlayState *state) {
    AWOverlayConfig c = state.config;
    const CGFloat weights[] = {NSFontWeightUltraLight, NSFontWeightThin, NSFontWeightLight,
        NSFontWeightRegular, NSFontWeightMedium, NSFontWeightSemibold, NSFontWeightBold,
        NSFontWeightHeavy, NSFontWeightBlack};
    double slot = (c.weight - 100) / 100.0;
    int lower = MIN(7, (int)slot);
    CGFloat weight = weights[lower] + (weights[lower + 1] - weights[lower]) * (slot - lower);
    NSFontDescriptor *descriptor = state.family.length
        ? [NSFontDescriptor fontDescriptorWithFontAttributes:@{NSFontFamilyAttribute:state.family}]
        : [NSFont systemFontOfSize:c.font_size weight:weight].fontDescriptor;
    descriptor = [descriptor fontDescriptorByAddingAttributes:@{NSFontTraitsAttribute:@{
        NSFontWeightTrait:@(weight), NSFontSymbolicTrait:@(c.italic ? NSFontItalicTrait : 0)}}];
    return [NSFont fontWithDescriptor:descriptor size:c.font_size];
}

@interface AWOverlay : NSObject
@property(nonatomic, strong) AWOverlayPanel *panel;
@property(nonatomic, strong) AWOverlayText *label;
@property(nonatomic, strong) NSNumber *displayID;
@property(nonatomic, strong) AWOverlayState *state;
- (BOOL)apply:(AWOverlayState *)state;
- (void)reposition:(NSNotification *)notification;
- (void)layoutContent;
@end

@implementation AWOverlay
- (void)layoutContent {
    AWOverlayConfig c = self.state.config;
    NSSize size = self.panel.contentView.bounds.size;
    CGFloat left = MIN(c.left, size.width), top = MIN(c.top, size.height);
    CGFloat width = MAX(0, size.width - left - MIN(c.right, size.width - left));
    CGFloat height = MAX(0, size.height - top - MIN(c.bottom, size.height - top));
    // 手动饱和布局允许内边距超过窗口，不创建互相冲突的约束。
    NSRect frame = NSMakeRect(left, size.height - top - height, width, height);
    if (!NSEqualRects(self.label.frame, frame)) {
        self.label.frame = frame;
        self.label.needsDisplay = YES;
    }
}
- (void)reposition:(NSNotification *)notification {
    NSScreen *screen = screenForID(self.displayID.unsignedIntValue) ?: screenForID(0);
    // 临时回退不覆盖原始显示器身份，重连后自动恢复。
    if (!screen) { [self.panel orderOut:nil]; return; }
    AWOverlayConfig c = self.state.config;
    NSRect bounds = screen.frame;
    CGFloat width = MIN(c.width, NSWidth(bounds)), height = MIN(c.height, NSHeight(bounds));
    CGFloat x = axisOffset(c.anchor % 3, c.x, NSWidth(bounds) - width);
    CGFloat y = axisOffset(c.anchor / 3, c.y, NSHeight(bounds) - height);
    NSRect frame = NSMakeRect(NSMinX(bounds) + x, NSMaxY(bounds) - y - height, width, height);
    if (!NSEqualRects(self.panel.frame, frame)) [self.panel setFrame:frame display:YES];
    [self layoutContent];
    if (!self.panel.visible) [self.panel orderFrontRegardless];
}
- (BOOL)apply:(AWOverlayState *)state {
    AWOverlayState *old = self.state;
    AWOverlayConfig c = state.config, prior = old.config;
    BOOL selection = !old || c.display != prior.display;
    NSScreen *screen = selection ? screenForID(c.display) : nil;
    if (selection && !screen) return NO;
    BOOL fontChanged = !old || c.font_size != prior.font_size || c.weight != prior.weight ||
        c.italic != prior.italic || ![state.family isEqualToString:old.family];
    NSFont *font = fontChanged ? fontForState(state) : nil;
    if (fontChanged && !font) return NO;
    self.state = state;
    if (selection) self.displayID = screen.deviceDescription[@"NSScreenNumber"];
    if (!old || ![state.text isEqualToString:old.text]) {
        self.label.text = state.text;
        self.label.needsDisplay = YES;
    }
    if (fontChanged) { self.label.font = font; self.label.needsDisplay = YES; }
    if (!old || c.text_alpha != prior.text_alpha) {
        self.label.color = [NSColor colorWithWhite:1 alpha:c.text_alpha];
        self.label.needsDisplay = YES;
    }
    if (!old || c.background_alpha != prior.background_alpha)
        self.panel.backgroundColor = [NSColor colorWithWhite:0 alpha:c.background_alpha];
    if (selection || c.anchor != prior.anchor || c.x != prior.x || c.y != prior.y ||
        c.width != prior.width || c.height != prior.height) {
        [self reposition:nil];
    } else if (c.top != prior.top || c.right != prior.right || c.bottom != prior.bottom || c.left != prior.left) {
        [self layoutContent];
    }
    return YES;
}
@end

static AWOverlay *overlay;
static NSLock *updateLock;
static AWOverlayState *pendingState;
static BOOL updateScheduled;
static BOOL acceptingUpdates;
static unsigned int submittedDisplay;
static int runStatus;

static void stopApplication(void) {
    [NSApp stop:nil];
    // 唤醒无用户输入时阻塞的原生事件循环。
    [NSApp postEvent:[NSEvent otherEventWithType:NSEventTypeApplicationDefined
        location:NSZeroPoint modifierFlags:0 timestamp:0 windowNumber:0
        context:nil subtype:0 data1:0 data2:0] atStart:YES];
}

int overlay_create(AWOverlayConfig config) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        if (!screenForID(config.display)) return 0;
        overlay = [AWOverlay new];
        AWOverlayPanel *panel = [[AWOverlayPanel alloc]
            initWithContentRect:NSMakeRect(0, 0, 1, 1)
            styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
            backing:NSBackingStoreBuffered defer:NO];
        if (!panel) { overlay = nil; return 0; }
        overlay.panel = panel;
        panel.title = @"Arcana World Overlay";
        panel.releasedWhenClosed = NO;
        panel.opaque = NO;
        panel.hasShadow = NO;
        panel.ignoresMouseEvents = YES;
        panel.hidesOnDeactivate = NO;
        panel.movable = NO;
        panel.animationBehavior = NSWindowAnimationBehaviorNone;
        panel.level = NSStatusWindowLevel + 1;
        panel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces |
            NSWindowCollectionBehaviorFullScreenAuxiliary | NSWindowCollectionBehaviorStationary |
            NSWindowCollectionBehaviorIgnoresCycle;
        overlay.label = [[AWOverlayText alloc] initWithFrame:NSZeroRect];
        [panel.contentView addSubview:overlay.label];
        if (![overlay apply:copyState(config)]) { [panel close]; overlay = nil; return 0; }
        updateLock = [NSLock new];
        acceptingUpdates = YES;
        updateScheduled = NO;
        submittedDisplay = config.display;
        runStatus = 1;
        [NSNotificationCenter.defaultCenter addObserver:overlay selector:@selector(reposition:)
            name:NSApplicationDidChangeScreenParametersNotification object:nil];
        [NSWorkspace.sharedWorkspace.notificationCenter addObserver:overlay selector:@selector(reposition:)
            name:NSWorkspaceActiveSpaceDidChangeNotification object:nil];
        [NSWorkspace.sharedWorkspace.notificationCenter addObserver:overlay selector:@selector(reposition:)
            name:NSWorkspaceDidWakeNotification object:nil];
        [panel displayIfNeeded];
        return 1;
    }
}

int overlay_update(AWOverlayConfig config) {
    @autoreleasepool {
        AWOverlayState *state = copyState(config);
        [updateLock lock];
        if (!acceptingUpdates) { [updateLock unlock]; return 1; }
        // CoreGraphics 的显示器查询可在生产者线程执行，拒绝被后续快照覆盖的无效选择。
        if (config.display != submittedDisplay && config.display && !CGDisplayIsOnline(config.display)) {
            [updateLock unlock];
            return 0;
        }
        submittedDisplay = config.display;
        pendingState = state;
        if (updateScheduled) { [updateLock unlock]; return 1; }
        updateScheduled = YES;
        [updateLock unlock];
        dispatch_async(dispatch_get_main_queue(), ^{
            [updateLock lock];
            AWOverlayState *latest = acceptingUpdates ? pendingState : nil;
            pendingState = nil;
            updateScheduled = NO;
            [updateLock unlock];
            if (overlay && latest && ![overlay apply:latest]) {
                runStatus = 0;
                stopApplication();
            }
        });
        return 1;
    }
}

void overlay_stop(void) {
    [updateLock lock];
    acceptingUpdates = NO;
    pendingState = nil;
    [updateLock unlock];
    dispatch_async(dispatch_get_main_queue(), ^{ stopApplication(); });
}

int overlay_run(void) {
    @autoreleasepool {
        [NSApp run];
        [updateLock lock];
        acceptingUpdates = NO;
        pendingState = nil;
        [updateLock unlock];
        [NSNotificationCenter.defaultCenter removeObserver:overlay];
        [NSWorkspace.sharedWorkspace.notificationCenter removeObserver:overlay];
        [overlay.panel close];
        overlay = nil;
        return runStatus;
    }
}
