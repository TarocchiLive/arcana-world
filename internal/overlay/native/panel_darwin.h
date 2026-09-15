//go:build darwin && cgo

#ifndef AW_OVERLAY_DARWIN_H
#define AW_OVERLAY_DARWIN_H

typedef struct {
    const char *text;
    const char *family;
    double x, y, width, height;
    double top, right, bottom, left;
    double font_size, text_alpha, background_alpha;
    int anchor, weight, italic;
    unsigned int display;
} AWOverlayConfig;

int overlay_create(AWOverlayConfig config);
int overlay_update(AWOverlayConfig config);
int overlay_run(void);
void overlay_stop(void);
#endif
