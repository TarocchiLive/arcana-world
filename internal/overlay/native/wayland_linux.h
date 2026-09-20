#ifndef ARCANA_WAYLAND_LINUX_H
#define ARCANA_WAYLAND_LINUX_H
#include <stddef.h>
#include <stdint.h>

struct arcana_wayland_text_run
{
    size_t start, end;
    uint32_t rgb;
};

struct arcana_wayland_config
{
    char *text, *output, *family, *displays;
    size_t displays_size;
    struct arcana_wayland_text_run *text_runs;
    size_t text_run_count;
    uint32_t text_rgb, background_rgb;
    int explicit_displays, anchor, weight, italic, outline;
    double x, y, width, height, padding_top, padding_right, padding_bottom, padding_left;
    double font_size, text_alpha, background_alpha;
};
// run 独占 Wayland 对象；update/stop 只操作有界 mailbox，字符串和文字颜色区间深复制。
struct arcana_wayland;
struct arcana_wayland *arcana_wayland_new(const struct arcana_wayland_config *, uintptr_t ready_handle);
int arcana_wayland_update(struct arcana_wayland *, const struct arcana_wayland_config *);
void arcana_wayland_stop(struct arcana_wayland *);
const char *arcana_wayland_run(struct arcana_wayland *);
void arcana_wayland_free(struct arcana_wayland *);
const char *arcana_wayland_list(struct arcana_wayland *, uintptr_t handle);
void arcanaWaylandDisplay(uintptr_t handle, char *id, char *name);
void arcanaWaylandReady(uintptr_t handle);
#endif
