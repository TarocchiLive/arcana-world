//go:build darwin && cgo

#ifndef AW_OVERLAY_DARWIN_H
#define AW_OVERLAY_DARWIN_H

#include <stddef.h>
#include <stdint.h>

typedef struct {
    size_t start, end;
    uint32_t rgb;
} AWOverlayTextRun;

typedef struct {
    const char *text;
    const char *family;
    const char *displays;
    size_t text_length;
    const AWOverlayTextRun *runs;
    size_t run_count;
    uint32_t text_rgb, background_rgb;
    double x, y, width, height;
    double top, right, bottom, left;
    double font_size, text_alpha, background_alpha;
    int anchor, weight, italic, outline;
    unsigned int display;
} AWOverlayConfig;

int overlay_create(AWOverlayConfig config);
int overlay_update(AWOverlayConfig config);
int overlay_run(void);
void overlay_stop(void);
char *overlay_list_displays(unsigned int legacyDisplay);
#endif
