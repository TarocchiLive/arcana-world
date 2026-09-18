#ifndef ARCANA_WAYLAND_LINUX_H
#define ARCANA_WAYLAND_LINUX_H
#include <stddef.h>
#include <stdint.h>

struct arcana_wayland_config
{
    char *text, *output, *family;
    int anchor, weight, italic;
    double x, y, width, height, padding_top, padding_right, padding_bottom, padding_left;
    double font_size, text_alpha, background_alpha;
};
// run 独占 Wayland 对象；update/stop 只操作有界 mailbox，所有字符串深复制。
struct arcana_wayland;
struct arcana_wayland *arcana_wayland_new(const struct arcana_wayland_config *, uintptr_t ready_handle);
int arcana_wayland_update(struct arcana_wayland *, const struct arcana_wayland_config *);
void arcana_wayland_stop(struct arcana_wayland *);
const char *arcana_wayland_run(struct arcana_wayland *);
void arcana_wayland_free(struct arcana_wayland *);
void arcanaWaylandReady(uintptr_t handle);
#endif
