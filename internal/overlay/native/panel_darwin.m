//go:build darwin && cgo

#import <Cocoa/Cocoa.h>
#import <CoreGraphics/CoreGraphics.h>
#import "panel_darwin.h"
#include <string.h>

// 所有窗口、字体和绘制状态只由主线程持有；生产者只写最新快照。
@interface AWOverlayState : NSObject
@property AWOverlayConfig config;
@property(nonatomic, copy) NSString *text;
@property(nonatomic, copy) NSString *family;
@property(nonatomic, copy) NSArray<NSString *> *displays;
@property(nonatomic, copy) NSData *runs;
@property(nonatomic, strong) NSFont *font;
@end
@implementation AWOverlayState
@end

static AWOverlayState *copyState(AWOverlayConfig config) {
    AWOverlayState *state = [AWOverlayState new];
    state.text = [[NSString alloc] initWithBytes:config.text length:config.text_length encoding:NSUTF8StringEncoding];
    state.family = [NSString stringWithUTF8String:config.family];
    if (!state.text || !state.family) return nil;
    if (config.run_count > NSUIntegerMax / sizeof(AWOverlayTextRun) ||
        (config.run_count && !config.runs)) return nil;
    NSMutableData *runs = [NSMutableData dataWithLength:config.run_count * sizeof(AWOverlayTextRun)];
    AWOverlayTextRun *converted = runs.mutableBytes;
    const unsigned char *text = (const unsigned char *)config.text;
    size_t byteOffset = 0, utf16Offset = 0;
    for (size_t i = 0; i < config.run_count; ++i) {
        AWOverlayTextRun run = config.runs[i];
        if (run.start != byteOffset || run.end < run.start || run.end > config.text_length ||
            (run.end < config.text_length && (text[run.end] & 0xC0) == 0x80)) return nil;
        converted[i].start = utf16Offset;
        // NSString 已验证 UTF-8；每个非续字节对应一个标量，非 BMP 标量占两个 UTF-16 单元。
        while (byteOffset < run.end) {
            unsigned char byte = text[byteOffset++];
            if ((byte & 0xC0) != 0x80) utf16Offset += byte >= 0xF0 ? 2 : 1;
        }
        converted[i].end = utf16Offset;
        converted[i].rgb = run.rgb;
    }
    if (config.run_count && byteOffset != config.text_length) return nil;
    state.runs = runs;
    if (config.displays) {
        NSData *data = [[NSString stringWithUTF8String:config.displays] dataUsingEncoding:NSUTF8StringEncoding];
        id displays = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
        if (![displays isKindOfClass:NSArray.class]) return nil;
        for (id display in displays)
            if (![display isKindOfClass:NSString.class] || ![display length]) return nil;
        state.displays = displays;
    }
    config.displays = NULL;
    config.text = NULL;
    config.family = NULL;
    config.runs = NULL;
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

static NSColor *colorForRGB(uint32_t rgb, CGFloat alpha) {
    return [NSColor colorWithSRGBRed:((rgb >> 16) & 0xFF) / 255.0
        green:((rgb >> 8) & 0xFF) / 255.0 blue:(rgb & 0xFF) / 255.0 alpha:alpha];
}

@interface AWOverlayText : NSView
@property(nonatomic, copy) NSString *text;
@property(nonatomic, strong) NSFont *font;
@property(nonatomic, strong) NSColor *color;
@property(nonatomic, copy) NSData *runs;
@property(nonatomic) BOOL outline;
@end
@implementation AWOverlayText {
    NSTextStorage *_storage;
    NSLayoutManager *_layout;
    NSTextContainer *_container;
    NSString *_laidOutText;
    NSFont *_laidOutFont;
    NSColor *_laidOutColor;
    NSData *_laidOutRuns;
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
        ![_laidOutColor isEqual:self.color] || ![_laidOutRuns isEqualToData:self.runs]) {
        // 按配置字体的逻辑点取整行高，避免回退字体或 Retina 缩放改变行容量。
        _lineHeight = ceil([_layout defaultLineHeightForFont:self.font]);
        _baseline = self.font.ascender +
            MAX(0, (_lineHeight - self.font.ascender + self.font.descender) / 2);
        NSMutableParagraphStyle *paragraph = [NSMutableParagraphStyle new];
        paragraph.lineBreakMode = NSLineBreakByWordWrapping;
        paragraph.minimumLineHeight = _lineHeight;
        paragraph.maximumLineHeight = _lineHeight;
        [_storage beginEditing];
        [_storage setAttributedString:[[NSAttributedString alloc] initWithString:self.text attributes:@{
            NSFontAttributeName:self.font,
            NSForegroundColorAttributeName:self.color,
            NSParagraphStyleAttributeName:paragraph
        }]];
        const AWOverlayTextRun *runs = self.runs.bytes;
        NSUInteger count = self.runs.length / sizeof(AWOverlayTextRun);
        for (NSUInteger i = 0; i < count; ++i) {
            AWOverlayTextRun run = runs[i];
            [_storage addAttribute:NSForegroundColorAttributeName
                value:colorForRGB(run.rgb, self.color.alphaComponent)
                range:NSMakeRange(run.start, run.end - run.start)];
        }
        [_storage endEditing];
        _laidOutText = [self.text copy];
        _laidOutFont = self.font;
        _laidOutColor = self.color;
        _laidOutRuns = self.runs;
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
    [NSGraphicsContext saveGraphicsState];
    NSRectClip(bounds);
    NSRange textRange = NSMakeRange(0, _storage.length);
    // 先绘制描边；单独的填充绘制阶段可保留 CJK 字形的细笔画。
    for (NSUInteger pass = self.outline ? 0 : 1; pass < 2; ++pass) {
        [_storage beginEditing];
        if (pass == 0) {
            [_storage addAttributes:@{
                NSStrokeColorAttributeName:colorForRGB(0x101318, self.color.alphaComponent),
                NSStrokeWidthAttributeName:@(100.0 / self.font.pointSize)
            } range:textRange];
        } else {
            [_storage removeAttribute:NSStrokeWidthAttributeName range:textRange];
            [_storage removeAttribute:NSStrokeColorAttributeName range:textRange];
        }
        [_storage endEditing];
        __block NSUInteger index = 0;
        [_layout enumerateLineFragmentsForGlyphRange:allGlyphs usingBlock:
            ^(NSRect rect, NSRect used, NSTextContainer *container, NSRange glyphs, BOOL *stop) {
                NSUInteger row = index++;
                if (row < first || !glyphs.length) return;
                CGFloat top = NSMinY(bounds) + (row - first) * self->_lineHeight;
                NSPoint location = [self->_layout locationForGlyphAtIndex:glyphs.location];
                NSPoint origin = NSMakePoint(NSMinX(bounds),
                    top + self->_baseline - NSMinY(rect) - location.y);
                [NSGraphicsContext saveGraphicsState];
                NSRectClip(NSMakeRect(NSMinX(bounds), top, NSWidth(bounds), self->_lineHeight));
                [self->_layout drawGlyphsForGlyphRange:glyphs atPoint:origin];
                [NSGraphicsContext restoreGraphicsState];
            }];
    }
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

static NSString *screenUUID(NSScreen *screen) {
    CGDirectDisplayID display = [screen.deviceDescription[@"NSScreenNumber"] unsignedIntValue];
    CFUUIDRef uuid = CGDisplayCreateUUIDFromDisplayID(display);
    if (!uuid) return nil;
    NSString *identifier = CFBridgingRelease(CFUUIDCreateString(kCFAllocatorDefault, uuid));
    CFRelease(uuid);
    return identifier;
}

char *overlay_list_displays(unsigned int legacyDisplay) {
    @autoreleasepool {
        if (![NSThread isMainThread]) return NULL;
        [NSApplication sharedApplication];
        if (!NSScreen.screens.count) return NULL;
        NSMutableArray *displays = [NSMutableArray new];
        NSScreen *legacy = screenForID(legacyDisplay);
        for (NSScreen *screen in NSScreen.screens) {
            NSString *identifier = screenUUID(screen);
            if (!identifier) return NULL;
            [displays addObject:@{@"id":identifier, @"name":screen.localizedName,
                @"selected":screen == legacy ? @YES : @NO}];
        }
        NSData *data = [NSJSONSerialization dataWithJSONObject:displays options:0 error:nil];
        if (!data) return NULL;
        return strdup([[NSString alloc] initWithData:data encoding:NSUTF8StringEncoding].UTF8String);
    }
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
@property(nonatomic, strong) NSScreen *screen;
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
    NSRect frame = NSMakeRect(left, size.height - top - height, width, height);
    if (!NSEqualRects(self.label.frame, frame)) {
        self.label.frame = frame;
        self.label.needsDisplay = YES;
    }
}
- (void)reposition:(NSNotification *)notification {
    if (!self.screen) { [self.panel orderOut:nil]; return; }
    AWOverlayConfig c = self.state.config;
    NSRect bounds = self.screen.frame;
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
    BOOL fontChanged = !old || ![state.font isEqual:old.font];
    self.state = state;
    if (!old || ![state.text isEqualToString:old.text]) {
        self.label.text = state.text;
        self.label.needsDisplay = YES;
    }
    if (fontChanged) { self.label.font = state.font; self.label.needsDisplay = YES; }
    if (!old || ![state.runs isEqualToData:old.runs]) {
        self.label.runs = state.runs;
        self.label.needsDisplay = YES;
    }
    if (!old || c.text_alpha != prior.text_alpha || c.text_rgb != prior.text_rgb) {
        self.label.color = colorForRGB(c.text_rgb, c.text_alpha);
        self.label.needsDisplay = YES;
    }
    if (!old || c.outline != prior.outline) {
        self.label.outline = c.outline != 0;
        self.label.needsDisplay = YES;
    }
    if (!old || c.background_alpha != prior.background_alpha || c.background_rgb != prior.background_rgb)
        self.panel.backgroundColor = colorForRGB(c.background_rgb, c.background_alpha);
    if (!old || c.anchor != prior.anchor || c.x != prior.x || c.y != prior.y ||
        c.width != prior.width || c.height != prior.height) {
        [self reposition:nil];
    } else if (c.top != prior.top || c.right != prior.right || c.bottom != prior.bottom || c.left != prior.left) {
        [self layoutContent];
    }
    return YES;
}
@end

static int runStatus;
@interface AWOverlayGroup : NSObject
@property(nonatomic, strong) NSMutableDictionary<NSString *, AWOverlay *> *windows;
@property(nonatomic, strong) AWOverlayState *state;
@property(nonatomic, copy) NSString *legacyDisplay;
- (BOOL)apply:(AWOverlayState *)state;
- (BOOL)apply:(AWOverlayState *)state reconcile:(BOOL)reconcile;
- (void)reposition:(NSNotification *)notification;
- (void)close;
@end

static AWOverlayPanel *newPanel(void) {
    AWOverlayPanel *panel = [[AWOverlayPanel alloc]
        initWithContentRect:NSMakeRect(0, 0, 1, 1)
        styleMask:NSWindowStyleMaskBorderless | NSWindowStyleMaskNonactivatingPanel
        backing:NSBackingStoreBuffered defer:NO];
    panel.title = @"Arcana World Overlay";
    panel.releasedWhenClosed = NO;
    panel.opaque = NO;
    panel.backgroundColor = NSColor.clearColor;
    panel.hasShadow = NO;
    panel.ignoresMouseEvents = YES;
    panel.hidesOnDeactivate = NO;
    panel.movable = NO;
    panel.animationBehavior = NSWindowAnimationBehaviorNone;
    panel.level = NSStatusWindowLevel + 1;
    panel.collectionBehavior = NSWindowCollectionBehaviorCanJoinAllSpaces |
        NSWindowCollectionBehaviorFullScreenAuxiliary | NSWindowCollectionBehaviorStationary |
        NSWindowCollectionBehaviorIgnoresCycle;
    return panel;
}

@implementation AWOverlayGroup
- (BOOL)apply:(AWOverlayState *)state {
    return [self apply:state reconcile:NO];
}
- (BOOL)apply:(AWOverlayState *)state reconcile:(BOOL)reconcile {
    if (!state) return NO;
    AWOverlayConfig c = state.config, prior = self.state.config;
    if (self.state && c.font_size == prior.font_size && c.weight == prior.weight &&
        c.italic == prior.italic && [state.family isEqualToString:self.state.family])
        state.font = self.state.font;
    else
        state.font = fontForState(state);
    if (!state.font) return NO;
    BOOL sameSelection = self.state &&
        ((state.displays && [state.displays isEqualToArray:self.state.displays]) ||
         (!state.displays && !self.state.displays && c.display == prior.display));
    if (!sameSelection && !state.displays) {
        NSScreen *screen = screenForID(c.display);
        NSString *identifier = screen ? screenUUID(screen) : nil;
        if (!identifier) return NO;
        self.legacyDisplay = identifier;
    }
    self.state = state;
    if (!reconcile && sameSelection) {
        for (NSString *identifier in self.windows)
            [self.windows[identifier] apply:state];
        return YES;
    }
    if (!self.windows) self.windows = [NSMutableDictionary new];
    NSMutableDictionary<NSString *, NSScreen *> *wanted = [NSMutableDictionary new];
    if (state.displays) {
        NSSet *selected = [NSSet setWithArray:state.displays];
        for (NSScreen *screen in NSScreen.screens) {
            NSString *identifier = screenUUID(screen);
            if (identifier && [selected containsObject:identifier]) wanted[identifier] = screen;
        }
    } else {
        NSScreen *screen = nil;
        for (NSScreen *candidate in NSScreen.screens) {
            if ([screenUUID(candidate) isEqualToString:self.legacyDisplay]) {
                screen = candidate;
                break;
            }
        }
        // 旧单屏模式临时回退，保留原始身份以便重连后恢复。
        if (!screen) screen = screenForID(0);
        NSString *identifier = screen ? screenUUID(screen) : nil;
        if (screen && !identifier) return NO;
        if (identifier) wanted[identifier] = screen;
    }
    for (NSString *identifier in self.windows.allKeys) {
        if (!wanted[identifier]) {
            [self.windows[identifier].panel close];
            [self.windows removeObjectForKey:identifier];
        }
    }
    for (NSString *identifier in wanted) {
        AWOverlay *window = self.windows[identifier];
        BOOL created = !window;
        if (created) {
            window = [AWOverlay new];
            window.panel = newPanel();
            if (!window.panel) return NO;
            window.label = [[AWOverlayText alloc] initWithFrame:NSZeroRect];
            [window.panel.contentView addSubview:window.label];
            self.windows[identifier] = window;
        }
        window.screen = wanted[identifier];
        [window apply:state];
        [window reposition:nil];
    }
    return YES;
}
- (void)reposition:(NSNotification *)notification {
    if (![self apply:self.state reconcile:YES]) {
        runStatus = 0;
        overlay_stop();
    }
}
- (void)close {
    for (AWOverlay *window in self.windows.allValues) [window.panel close];
    [self.windows removeAllObjects];
}
@end

static AWOverlayGroup *overlay;
static NSLock *updateLock;
static AWOverlayState *pendingState;
static BOOL updateScheduled;
static BOOL acceptingUpdates;

static void stopApplication(void) {
    [NSApp stop:nil];
    // 唤醒无用户输入时阻塞的原生事件循环。
    [NSApp postEvent:[NSEvent otherEventWithType:NSEventTypeApplicationDefined
        location:NSZeroPoint modifierFlags:0 timestamp:0 windowNumber:0
        context:nil subtype:0 data1:0 data2:0] atStart:YES];
}

int overlay_create(AWOverlayConfig config) {
    @autoreleasepool {
        if (![NSThread isMainThread]) return 0;
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        if (!NSScreen.screens.count) return 0;
        overlay = [AWOverlayGroup new];
        if (![overlay apply:copyState(config)]) { [overlay close]; overlay = nil; return 0; }
        updateLock = [NSLock new];
        acceptingUpdates = YES;
        updateScheduled = NO;
        runStatus = 1;
        [NSNotificationCenter.defaultCenter addObserver:overlay selector:@selector(reposition:)
            name:NSApplicationDidChangeScreenParametersNotification object:nil];
        [NSWorkspace.sharedWorkspace.notificationCenter addObserver:overlay selector:@selector(reposition:)
            name:NSWorkspaceActiveSpaceDidChangeNotification object:nil];
        [NSWorkspace.sharedWorkspace.notificationCenter addObserver:overlay selector:@selector(reposition:)
            name:NSWorkspaceDidWakeNotification object:nil];
        return 1;
    }
}

int overlay_update(AWOverlayConfig config) {
    @autoreleasepool {
        AWOverlayState *state = copyState(config);
        if (!state) return 0;
        [updateLock lock];
        if (!acceptingUpdates) { [updateLock unlock]; return 1; }
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
        [overlay close];
        overlay = nil;
        return runStatus;
    }
}
