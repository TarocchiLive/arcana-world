//go:build linux && cgo && wayland

#define _GNU_SOURCE
#include "wayland_linux.h"
#include "protocols/wayland_protocols.h"
#include <pango/pangocairo.h>
#include <errno.h>
#include <fcntl.h>
#include <limits.h>
#include <math.h>
#include <poll.h>
#include <pthread.h>
#include <stdarg.h>
#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/mman.h>
#include <time.h>
#include <unistd.h>

// 最多同时存在两个像素存储，每个不超过 64 MiB。忙碌存储即使在 configure、
// 缩放变化或 surface 重建期间也绝不复用。
#define MAX_BUFFER_BYTES (64u * 1024u * 1024u)
struct arcana_wayland;
struct output {
    struct output *next;
    struct arcana_wayland *state;
    struct wl_output *proxy;
    struct wl_proxy *xdg;
    uint32_t id;
    int scale, mode_width, mode_height, transform, logical_width, logical_height;
    char *name;
    bool ready;
};
struct pixels {
    struct wl_buffer *buffer;
    void *data;
    size_t size;
    int width, height, stride;
    bool busy;
};
struct arcana_wayland {
    // 只有 mailbox 和 stop 标志由 Go 的生产者 goroutine 共享。
    pthread_mutex_t mutex;
    int wake[2];
    bool stop;
    struct arcana_wayland_config *pending;
    // 以下内容仅限于 arcana_wayland_run 的分发线程。
    struct arcana_wayland_config *config;
    char *wanted;
    char error[384];
    uintptr_t ready_handle;
    bool ready_sent, updated, settle_outputs;
    int width, height;
    int effective_left, effective_top, effective_width, effective_height;
    struct wl_display *display;
    struct wl_registry *registry;
    struct wl_compositor *compositor;
    struct wl_shm *shm;
    struct wl_proxy *shell, *viewporter, *fractional_manager, *xdg_manager;
    struct wl_surface *surface;
    struct wl_proxy *layer, *viewport, *fractional;
    struct wl_callback *frame;
    struct output *outputs, *active;
    struct pixels pixels[2];
    uint32_t fractional_scale;
    bool configured, dirty, closed, initial, argb;
};
static void fail(struct arcana_wayland *s, const char *format, ...) {
    if (s->error[0]) return;
    va_list args;
    va_start(args, format);
    vsnprintf(s->error, sizeof(s->error), format, args);
    va_end(args);
}
static void destroy_extension(struct wl_proxy **proxy, uint32_t opcode) {
    if (!*proxy) return;
    wl_proxy_marshal(*proxy, opcode);
    wl_proxy_destroy(*proxy);
    *proxy = NULL;
}

// 跨线程 mailbox：只有一个可替换更新，不是已渲染帧的队列。
static void wake(struct arcana_wayland *s) {
    char c = 1;
    // EAGAIN 表示唤醒已在队列中。本线程不调用 Wayland。
    while (write(s->wake[1], &c, 1) < 0 && errno == EINTR) {}
}
static void free_config(struct arcana_wayland_config *config) {
    if (!config) return;
    free(config->text); free(config->output); free(config->family); free(config);
}
static struct arcana_wayland_config *copy_config(const struct arcana_wayland_config *value) {
    struct arcana_wayland_config *copy = malloc(sizeof(*copy));
    if (!copy) return NULL;
    *copy = *value;
    copy->text = strdup(value->text); copy->output = strdup(value->output); copy->family = strdup(value->family);
    if (!copy->text || !copy->output || !copy->family) { free_config(copy); return NULL; }
    return copy;
}
// 位置和输出变化只需更新合成器状态，不必重新整形文字或绘制像素。
static bool equal_paint_config(const struct arcana_wayland_config *a, const struct arcana_wayland_config *b) {
    return !strcmp(a->text,b->text) && !strcmp(a->family,b->family) &&
        a->weight==b->weight && a->italic==b->italic &&
        a->padding_top==b->padding_top && a->padding_right==b->padding_right &&
        a->padding_bottom==b->padding_bottom && a->padding_left==b->padding_left &&
        a->font_size==b->font_size && a->text_alpha==b->text_alpha && a->background_alpha==b->background_alpha;
}
static bool equal_config(const struct arcana_wayland_config *a, const struct arcana_wayland_config *b) {
    return equal_paint_config(a,b) && !strcmp(a->output,b->output) &&
        a->anchor==b->anchor && a->x==b->x && a->y==b->y &&
        a->width==b->width && a->height==b->height;
}
struct arcana_wayland *arcana_wayland_new(const struct arcana_wayland_config *config, uintptr_t ready_handle) {
    struct arcana_wayland *s = calloc(1, sizeof(*s));
    if (!s) return NULL;
    if (pthread_mutex_init(&s->mutex, NULL)) { free(s); return NULL; }
    if (pipe2(s->wake, O_CLOEXEC | O_NONBLOCK)) {
        pthread_mutex_destroy(&s->mutex); free(s); return NULL;
    }
    s->config = copy_config(config);
    s->wanted = strdup(config->output);
    if (!s->config || !s->wanted) { arcana_wayland_free(s); return NULL; }
    s->ready_handle = ready_handle;
    s->initial = true;
    return s;
}
int arcana_wayland_update(struct arcana_wayland *s, const struct arcana_wayland_config *config) {
    struct arcana_wayland_config *copy = copy_config(config);
    if (!copy) return 0;
    pthread_mutex_lock(&s->mutex);
    bool notify = !s->pending;
    free_config(s->pending); s->pending = copy;
    pthread_mutex_unlock(&s->mutex);
    if (notify) wake(s);
    return 1;
}
void arcana_wayland_stop(struct arcana_wayland *s) {
    pthread_mutex_lock(&s->mutex); s->stop = true; pthread_mutex_unlock(&s->mutex);
    wake(s);
}
static bool consume(struct arcana_wayland *s) {
    char bytes[128];
    // 有界读取一次即可：如果生产者抢先写入，管道仍保持可读。
    (void)read(s->wake[0], bytes, sizeof(bytes));
    pthread_mutex_lock(&s->mutex);
    bool stop = s->stop;
    struct arcana_wayland_config *next = s->pending;
    s->pending = NULL;
    pthread_mutex_unlock(&s->mutex);
    if (next && !equal_config(next, s->config)) {
        if (strcmp(next->output, s->config->output)) {
            char *wanted = strdup(next->output);
            if (!wanted) fail(s, "cannot allocate output selector");
            else { free(s->wanted); s->wanted = wanted; s->initial = true; s->settle_outputs = true; }
        }
        if (!equal_paint_config(next, s->config)) s->dirty = true;
        free_config(s->config); s->config = next; s->updated = true;
    } else free_config(next);
    return stop;
}

// 显示 I/O：调用方传入失败操作捕获的错误。
static void display_error(struct arcana_wayland *s, int io_error) {
    int code = wl_display_get_error(s->display);
    if (code == EPROTO) {
        const struct wl_interface *interface = NULL;
        uint32_t id = 0;
        uint32_t protocol = wl_display_get_protocol_error(s->display, &interface, &id);
        fail(s, "protocol error %u on %s@%u", protocol, interface ? interface->name : "unknown", id);
    } else fail(s, "compositor disconnected or I/O failed: %s", strerror(code ? code : io_error));
}
// 事件返回 0，取消返回 1，错误返回 -1。prepare/read 配对保持分发单线程化，
// 并使取消即使在启动阶段也能中断。
static int pump(struct arcana_wayland *s, int timeout) {
    if (consume(s)) return 1;
    if (s->updated) { s->updated = false; return 0; }
    if (s->error[0]) return -1;
    if (wl_display_prepare_read(s->display) != 0) {
        if (wl_display_dispatch_pending(s->display) < 0) { display_error(s, errno); return -1; }
        if (consume(s)) return 1;
        return s->error[0] ? -1 : 0;
    }
    int flush = wl_display_flush(s->display);
    if (flush < 0 && errno != EAGAIN) {
        int code = errno;
        wl_display_cancel_read(s->display); display_error(s, code); return -1;
    }
    struct pollfd fds[2] = {
        {wl_display_get_fd(s->display), POLLIN | (flush < 0 ? POLLOUT : 0), 0},
        {s->wake[0], POLLIN, 0}
    };
    int result = poll(fds, 2, timeout);
    if (result < 0) {
        int code = errno;
        wl_display_cancel_read(s->display);
        if (code == EINTR) return 0;
        fail(s, "poll failed: %s", strerror(code)); return -1;
    }
    if (fds[0].revents & POLLIN) {
        if (wl_display_read_events(s->display) < 0) { display_error(s, errno); return -1; }
    } else wl_display_cancel_read(s->display);
    // poll 通过 revents 而非 errno 报告失败。prepare_read/flush 可能留下
    // EAGAIN；报告该错误会掩盖实际的合成器挂断。
    if (fds[0].revents & (POLLERR | POLLHUP | POLLNVAL)) {
        display_error(s, fds[0].revents & POLLNVAL ? EBADF : EPIPE); return -1;
    }
    if (wl_display_dispatch_pending(s->display) < 0) { display_error(s, errno); return -1; }
    if (consume(s)) return 1;
    return s->error[0] ? -1 : 0;
}
struct sync_state { bool done; };
static void sync_done(void *data, struct wl_callback *callback, uint32_t serial) {
    (void)callback; (void)serial; ((struct sync_state *)data)->done = true;
}
static const struct wl_callback_listener sync_listener = {sync_done};
static int synchronize(struct arcana_wayland *s) {
    struct sync_state sync = {false};
    struct wl_callback *callback = wl_display_sync(s->display);
    if (!callback) { fail(s, "cannot allocate display sync"); return -1; }
    wl_callback_add_listener(callback, &sync_listener, &sync);
    struct timespec start, now;
    clock_gettime(CLOCK_MONOTONIC, &start);
    int result = 0;
    while (!sync.done) {
        clock_gettime(CLOCK_MONOTONIC, &now);
        int64_t elapsed = (int64_t)(now.tv_sec - start.tv_sec) * 1000 + (now.tv_nsec - start.tv_nsec) / 1000000;
        if (elapsed >= 5000) { fail(s, "compositor initialization timed out"); result = -1; break; }
        result = pump(s, (int)(5000 - elapsed));
        if (result || sync.done) break;
    }
    wl_callback_destroy(callback);
    return result;
}

// Registry/output 的所有权。移除选中的 output 会清除借用的 active 指针；
// reconciliation 会在仍存活的 output 上重建 surface。
static void output_geometry(void *data, struct wl_output *p, int32_t x, int32_t y,
        int32_t pw, int32_t ph, int32_t sub, const char *make, const char *model, int32_t transform) {
    (void)p; (void)x; (void)y; (void)pw; (void)ph; (void)sub; (void)make; (void)model;
    struct output *o = data;
    o->transform = transform;
    o->state->dirty = true;
}
static void output_mode(void *data, struct wl_output *p, uint32_t flags, int32_t w, int32_t h, int32_t refresh) {
    (void)p; (void)refresh;
    if (!(flags & WL_OUTPUT_MODE_CURRENT)) return;
    struct output *o = data;
    if (w <= 0 || h <= 0) { fail(o->state, "invalid output mode dimensions"); return; }
    o->mode_width = w; o->mode_height = h; o->state->dirty = true;
}
static void output_done(void *data, struct wl_output *p) { (void)p; ((struct output *)data)->ready = true; }
static void output_scale(void *data, struct wl_output *p, int32_t scale) {
    (void)p; struct output *o = data;
    if (scale < 1 || scale > 64) { fail(o->state, "invalid or excessive output scale: %d", scale); return; }
    o->scale = scale; o->state->dirty = true;
}
static void set_output_name(struct output *o, const char *name) {
    char *copy = strdup(name);
    if (!copy) { fail(o->state, "cannot allocate output name"); return; }
    free(o->name); o->name = copy;
}
static void output_name(void *data, struct wl_output *p, const char *name) { (void)p; set_output_name(data, name); }
static void output_description(void *data, struct wl_output *p, const char *description) { (void)data; (void)p; (void)description; }
static const struct wl_output_listener output_listener = {
    output_geometry, output_mode, output_done, output_scale, output_name, output_description
};
static void xdg_position(void *data, struct wl_proxy *p, int32_t x, int32_t y) { (void)data; (void)p; (void)x; (void)y; }
static void xdg_size(void *data, struct wl_proxy *p, int32_t w, int32_t h) {
    (void)p; struct output *o = data;
    if (w <= 0 || h <= 0) { fail(o->state, "invalid logical output dimensions"); return; }
    o->logical_width = w; o->logical_height = h; o->state->dirty = true;
}
static void xdg_done(void *data, struct wl_proxy *p) { (void)p; ((struct output *)data)->ready = true; }
static void xdg_name(void *data, struct wl_proxy *p, const char *name) { (void)p; set_output_name(data, name); }
static void xdg_description(void *data, struct wl_proxy *p, const char *description) { (void)data; (void)p; (void)description; }
static void (*const xdg_listener[])(void) = {
    (void (*)(void))xdg_position, (void (*)(void))xdg_size, (void (*)(void))xdg_done,
    (void (*)(void))xdg_name, (void (*)(void))xdg_description
};
static void attach_xdg(struct arcana_wayland *s, struct output *o) {
    if (!s->xdg_manager || o->xdg) return;
    o->xdg = (struct wl_proxy *)wl_proxy_marshal_constructor_versioned(s->xdg_manager, 1,
        &zxdg_output_v1_interface, wl_proxy_get_version(s->xdg_manager), NULL, o->proxy);
    if (!o->xdg) { fail(s, "cannot allocate xdg-output"); return; }
    wl_proxy_add_listener(o->xdg, (void (**)(void))xdg_listener, o);
}
static void shm_format(void *data, struct wl_shm *shm, uint32_t format) {
    (void)shm;
    if (format == WL_SHM_FORMAT_ARGB8888) ((struct arcana_wayland *)data)->argb = true;
}
static const struct wl_shm_listener shm_listener = {shm_format};
static void registry_global(void *data, struct wl_registry *registry, uint32_t id, const char *name, uint32_t version) {
    struct arcana_wayland *s = data;
    if (!strcmp(name, "wl_compositor") && !s->compositor) {
        if (version < 3) { fail(s, "wl_compositor v3 or newer is required"); return; }
        s->compositor = wl_registry_bind(registry, id, &wl_compositor_interface, version < 4 ? version : 4);
    } else if (!strcmp(name, "wl_shm") && !s->shm) {
        s->shm = wl_registry_bind(registry, id, &wl_shm_interface, 1);
        if (!s->shm) { fail(s, "cannot bind shared-memory interface"); return; }
        wl_shm_add_listener(s->shm, &shm_listener, s);
    } else if (!strcmp(name, "zwlr_layer_shell_v1") && !s->shell) {
        s->shell = wl_registry_bind(registry, id, &zwlr_layer_shell_v1_interface, version < 4 ? version : 4);
    } else if (!strcmp(name, "wp_viewporter") && !s->viewporter) {
        s->viewporter = wl_registry_bind(registry, id, &wp_viewporter_interface, 1);
    } else if (!strcmp(name, "wp_fractional_scale_manager_v1") && !s->fractional_manager) {
        s->fractional_manager = wl_registry_bind(registry, id, &wp_fractional_scale_manager_v1_interface, 1);
    } else if (!strcmp(name, "zxdg_output_manager_v1") && !s->xdg_manager && version >= 2) {
        s->xdg_manager = wl_registry_bind(registry, id, &zxdg_output_manager_v1_interface, version < 3 ? version : 3);
        for (struct output *o = s->outputs; o; o = o->next) attach_xdg(s, o);
        s->settle_outputs = true;
    } else if (!strcmp(name, "wl_output")) {
        if (version < 2) { fail(s, "wl_output v2 or newer is required"); return; }
        struct output *o = calloc(1, sizeof(*o));
        if (!o) { fail(s, "cannot allocate output"); return; }
        o->state = s; o->id = id; o->scale = 1;
        o->proxy = wl_registry_bind(registry, id, &wl_output_interface, version < 4 ? version : 4);
        if (!o->proxy) { free(o); fail(s, "cannot bind output"); return; }
        // 保留 registry 顺序，以确定性地选择默认 output/回退 output。
        struct output **tail = &s->outputs;
        while (*tail) tail = &(*tail)->next;
        *tail = o;
        wl_output_add_listener(o->proxy, &output_listener, o);
        attach_xdg(s, o);
        s->settle_outputs = true;
    }
}
static void free_output(struct output *o) {
    destroy_extension(&o->xdg, 0);
    if (wl_output_get_version(o->proxy) >= 3) wl_output_release(o->proxy);
    else wl_output_destroy(o->proxy);
    free(o->name); free(o);
}
static void registry_remove(void *data, struct wl_registry *registry, uint32_t id) {
    (void)registry;
    struct arcana_wayland *s = data;
    struct output **link = &s->outputs;
    while (*link) {
        struct output *o = *link;
        if (o->id == id) {
            if (s->active == o) s->active = NULL;
            *link = o->next; free_output(o); return;
        }
        link = &o->next;
    }
}
static const struct wl_registry_listener registry_listener = {registry_global, registry_remove};
static void layer_configure(void *data, struct wl_proxy *layer, uint32_t serial, uint32_t width, uint32_t height) {
    struct arcana_wayland *s = data;
    wl_proxy_marshal(layer, 6, serial);
    if (width > 32768 || height > 32768) { fail(s, "compositor configured an excessive overlay size"); return; }
    int configured_width = width ? (int)width : s->effective_width;
    int configured_height = height ? (int)height : s->effective_height;
    if (!s->configured || s->width != configured_width || s->height != configured_height) s->dirty = true;
    s->width = configured_width; s->height = configured_height;
    s->configured = true;
}
static void layer_closed(void *data, struct wl_proxy *layer) { (void)layer; ((struct arcana_wayland *)data)->closed = true; }
static void (*const layer_listener[])(void) = {(void (*)(void))layer_configure, (void (*)(void))layer_closed};
static void preferred_scale(void *data, struct wl_proxy *proxy, uint32_t scale) {
    (void)proxy; struct arcana_wayland *s = data;
    if (!scale || scale > 7680) { fail(s, "invalid or excessive fractional scale: %u", scale); return; }
    s->fractional_scale = scale; s->dirty = true;
}
static void (*const fractional_listener[])(void) = {(void (*)(void))preferred_scale};

// Layer surface 生命周期与逻辑几何。
static void destroy_surface(struct arcana_wayland *s) {
    if (s->frame) { wl_callback_destroy(s->frame); s->frame = NULL; }
    destroy_extension(&s->fractional, 0);
    destroy_extension(&s->viewport, 0);
    destroy_extension(&s->layer, 7);
    if (s->surface) { wl_surface_destroy(s->surface); s->surface = NULL; }
    s->configured = false; s->closed = false; s->fractional_scale = 0;
}
static void surface_output_changed(void *data, struct wl_surface *surface, struct wl_output *output) {
    (void)surface; (void)output;
    ((struct arcana_wayland *)data)->dirty = true;
}
static const struct wl_surface_listener surface_listener = {
    .enter = surface_output_changed, .leave = surface_output_changed
};
static bool output_bounds(struct output *o, int *width, int *height) {
    // xdg-output 的逻辑尺寸已包含旋转和分数缩放。
    // 核心 wl_output 模式是物理像素，需应用这两项变换。
    if (o->logical_width > 0 && o->logical_height > 0) {
        *width = o->logical_width; *height = o->logical_height;
    } else {
        bool swapped = o->transform == WL_OUTPUT_TRANSFORM_90 ||
            o->transform == WL_OUTPUT_TRANSFORM_270 ||
            o->transform == WL_OUTPUT_TRANSFORM_FLIPPED_90 ||
            o->transform == WL_OUTPUT_TRANSFORM_FLIPPED_270;
        *width = (swapped ? o->mode_height : o->mode_width) / o->scale;
        *height = (swapped ? o->mode_width : o->mode_height) / o->scale;
    }
    return *width > 0 && *height > 0;
}
static double anchor_offset(int axis, double offset, double available) {
    if (axis == 1) offset = available / 2 + offset;
    else if (axis == 2) offset = available - offset;
    return fmax(0, fmin(offset, available));
}
static bool apply_geometry(struct arcana_wayland *s, bool initial) {
    int output_width, output_height;
    if (!s->active || !output_bounds(s->active, &output_width, &output_height)) {
        fail(s, "selected output has no usable full-output geometry"); return false;
    }
    const struct arcana_wayland_config *c = s->config;
    double bounded_width = fmin(c->width, output_width);
    double bounded_height = fmin(c->height, output_height);
    if (bounded_width > 32768 || bounded_height > 32768) {
        fail(s, "output-clamped overlay size exceeds native rendering limits"); return false;
    }
    int width = (int)fmax(1, round(bounded_width)), height = (int)fmax(1, round(bounded_height));
    int left = (int)round(anchor_offset(c->anchor % 3, c->x, output_width - width));
    int top = (int)round(anchor_offset(c->anchor / 3, c->y, output_height - height));
    if (!initial && width == s->effective_width && height == s->effective_height &&
        left == s->effective_left && top == s->effective_top) return false;
    bool resized = initial || width != s->effective_width || height != s->effective_height;
    s->effective_width = width; s->effective_height = height;
    s->effective_left = left; s->effective_top = top;
    // 统一使用左上锚点表达九宫格解析后的完整输出坐标，避免中心边距语义差异。
    wl_proxy_marshal(s->layer, 0, (uint32_t)width, (uint32_t)height);
    wl_proxy_marshal(s->layer, 3, top, 0, 0, left);
    if (resized) { s->configured = false; s->dirty = true; }
    return true;
}
static void create_surface(struct arcana_wayland *s, struct output *output) {
    s->active = output;
    s->surface = wl_compositor_create_surface(s->compositor);
    if (!s->surface) { fail(s, "cannot allocate surface"); return; }
    wl_surface_add_listener(s->surface, &surface_listener, s);
    s->layer = (struct wl_proxy *)wl_proxy_marshal_constructor_versioned(s->shell, 0,
        &zwlr_layer_surface_v1_interface, wl_proxy_get_version(s->shell), NULL,
        s->surface, output->proxy, 3u, "arcana-world-overlay");
    if (!s->layer) { fail(s, "cannot allocate layer surface"); return; }
    wl_proxy_add_listener(s->layer, (void (**)(void))layer_listener, s);
    wl_proxy_marshal(s->layer, 1, 1u | 4u); // 上边和左边。
    wl_proxy_marshal(s->layer, 2, -1); // 整个 output，绝不是工作区。
    wl_proxy_marshal(s->layer, 4, 0u); // keyboard_interactivity.none
    apply_geometry(s, true);
    if (s->error[0]) return;
    struct wl_region *empty = wl_compositor_create_region(s->compositor);
    if (!empty) { fail(s, "cannot allocate empty input region"); return; }
    wl_surface_set_input_region(s->surface, empty); // NULL 会接受输入。
    wl_region_destroy(empty);
    if (s->viewporter && s->fractional_manager) {
        s->viewport = (struct wl_proxy *)wl_proxy_marshal_constructor_versioned(s->viewporter, 1,
            &wp_viewport_interface, 1, NULL, s->surface);
        s->fractional = (struct wl_proxy *)wl_proxy_marshal_constructor_versioned(s->fractional_manager, 1,
            &wp_fractional_scale_v1_interface, 1, NULL, s->surface);
        if (!s->viewport || !s->fractional) { fail(s, "cannot allocate fractional scaling objects"); return; }
        wl_proxy_add_listener(s->fractional, (void (**)(void))fractional_listener, s);
    }
    s->dirty = true;
    wl_surface_commit(s->surface); // 初始空提交，在确认前不带 buffer。
}
static void reconcile_output(struct arcana_wayland *s) {
    struct output *selected = NULL, *fallback = NULL;
    for (struct output *o = s->outputs; o; o = o->next) {
        if (!o->ready) continue;
        if (!fallback) fallback = o;
        if (o->name && !strcmp(o->name, s->wanted)) selected = o;
    }
    if (s->initial) {
        if (*s->wanted && !selected) { fail(s, "output '%s' was not found (requires wl_output v4 or xdg-output v2 names)", s->wanted); return; }
        if (!selected) selected = fallback;
        if (!selected) { fail(s, "no Wayland output is available"); return; }
        if (!selected->name || !*selected->name) {
            fail(s, "stable output names require wl_output v4 or xdg-output v2"); return;
        }
        if (!*s->wanted && selected->name) {
            char *name = strdup(selected->name);
            if (!name) { fail(s, "cannot capture default output identity"); return; }
            free(s->wanted); s->wanted = name;
        }
        s->initial = false;
    }
    if (!selected) selected = s->active ? s->active : fallback;
    if (s->surface && (!s->active || s->active != selected)) destroy_surface(s);
    // 合成器可能在移除 output 时关闭 layer。事件循环会先同步，
    // 以便观察到相关的 registry 移除。
    if (s->closed) { fail(s, "compositor closed the overlay layer surface"); return; }
    if (!s->surface && selected) create_surface(s, selected);
    else if (s->surface && s->active && apply_geometry(s, false)) wl_surface_commit(s->surface);
}
static void buffer_release(void *data, struct wl_buffer *buffer) { (void)buffer; ((struct pixels *)data)->busy = false; }

// 像素存储的生命周期长于 surface。移除 output 后，合成器可能仍持有旧 surface
// 的 buffer；只有 wl_buffer.release 才允许复用。
static const struct wl_buffer_listener buffer_listener = {buffer_release};
static void free_pixels(struct pixels *p) {
    if (p->buffer) wl_buffer_destroy(p->buffer);
    if (p->data) munmap(p->data, p->size);
    memset(p, 0, sizeof(*p));
}
static bool allocate_pixels(struct arcana_wayland *s, struct pixels *p, int width, int height) {
    int stride = cairo_format_stride_for_width(CAIRO_FORMAT_ARGB32, width);
    if (stride <= 0 || height <= 0 || (size_t)height > MAX_BUFFER_BYTES / (size_t)stride) {
        fail(s, "scaled overlay backing store exceeds 64 MiB"); return false;
    }
    free_pixels(p);
    p->width = width; p->height = height; p->stride = stride; p->size = (size_t)stride * height;
    int fd = memfd_create("arcana-world-overlay", MFD_CLOEXEC);
    if (fd < 0) { fail(s, "memfd_create failed: %s", strerror(errno)); return false; }
    if (ftruncate(fd, (off_t)p->size)) { fail(s, "cannot size pixel store: %s", strerror(errno)); close(fd); return false; }
    p->data = mmap(NULL, p->size, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
    if (p->data == MAP_FAILED) { p->data = NULL; fail(s, "cannot map pixel store: %s", strerror(errno)); close(fd); return false; }
    struct wl_shm_pool *pool = wl_shm_create_pool(s->shm, fd, (int)p->size);
    close(fd);
    if (!pool) { fail(s, "cannot allocate shared-memory pool"); return false; }
    p->buffer = wl_shm_pool_create_buffer(pool, 0, width, height, stride, WL_SHM_FORMAT_ARGB8888);
    wl_shm_pool_destroy(pool);
    if (!p->buffer) { fail(s, "cannot allocate Wayland buffer"); return false; }
    wl_buffer_add_listener(p->buffer, &buffer_listener, p);
    return true;
}
static void frame_done(void *data, struct wl_callback *callback, uint32_t time) {
    (void)time; struct arcana_wayland *s = data;
    wl_callback_destroy(callback); s->frame = NULL;
}
static const struct wl_callback_listener frame_listener = {frame_done};

// 仅绘制到已释放的存储；呈现与帧节流在后续处理。
static bool paint_pixels(struct arcana_wayland *s, struct pixels *p) {
    // Cairo ARGB32 是本机字节序的预乘格式，与 wl_shm ARGB8888 匹配。
    cairo_surface_t *image = cairo_image_surface_create_for_data(p->data, CAIRO_FORMAT_ARGB32, p->width, p->height, p->stride);
    cairo_t *cr = cairo_create(image);
    cairo_scale(cr, (double)p->width / s->width, (double)p->height / s->height);
    cairo_set_operator(cr, CAIRO_OPERATOR_SOURCE);
    cairo_set_source_rgba(cr, 0, 0, 0, s->config->background_alpha);
    cairo_paint(cr);
    cairo_set_operator(cr, CAIRO_OPERATOR_OVER);
    const struct arcana_wayland_config *c = s->config;
    double left = fmin(c->padding_left, s->width), top = fmin(c->padding_top, s->height);
    double width = fmax(0, s->width - left - fmin(c->padding_right, s->width));
    double height = fmax(0, s->height - top - fmin(c->padding_bottom, s->height));
    if (width <= 0 || height <= 0 || !*c->text) goto done;
    cairo_rectangle(cr, left, top, width, height);
    cairo_clip(cr);
    PangoLayout *layout = pango_cairo_create_layout(cr);
    PangoFontDescription *font = pango_font_description_new();
    pango_font_description_set_family(font, *c->family ? c->family : "sans");
    pango_font_description_set_absolute_size(font, c->font_size * PANGO_SCALE);
    pango_font_description_set_weight(font, (PangoWeight)c->weight);
    pango_font_description_set_style(font, c->italic ? PANGO_STYLE_ITALIC : PANGO_STYLE_NORMAL);
    pango_layout_set_font_description(layout, font);
    pango_layout_set_width(layout, (int)(width * PANGO_SCALE));
    pango_layout_set_wrap(layout, PANGO_WRAP_WORD_CHAR);
    pango_layout_set_text(layout, c->text, -1);
    // 聊天快照按时间排列，最新内容在末尾。先按宽度完整换行，再在
    // 固定内容框内显示尾部；Pango 的高度省略会优先保留旧段落。
    int text_height;
    pango_layout_get_pixel_size(layout, NULL, &text_height);
    cairo_move_to(cr, left, top - fmax(0, text_height - height));
    cairo_set_source_rgba(cr, 1, 1, 1, c->text_alpha);
    pango_cairo_show_layout(cr, layout);
    pango_font_description_free(font);
    g_object_unref(layout);
done:
    cairo_surface_flush(image);
    cairo_status_t status = cairo_status(cr);
    if (status == CAIRO_STATUS_SUCCESS) status = cairo_surface_status(image);
    cairo_destroy(cr); cairo_surface_destroy(image);
    if (status != CAIRO_STATUS_SUCCESS) {
        fail(s, "Cairo drawing failed: %s", cairo_status_to_string(status)); return false;
    }
    return true;
}

// 渲染按帧节流，并且还需等待已释放的像素存储。
static void draw(struct arcana_wayland *s) {
    if (!s->dirty || !s->configured || s->frame || !s->active || s->error[0]) return;
    struct pixels *p = NULL;
    for (int i = 0; i < 2; ++i) if (!s->pixels[i].busy) { p = &s->pixels[i]; break; }
    if (!p) return;
    double scale = s->viewport && s->fractional_scale ? s->fractional_scale / 120.0 : s->active->scale;
    // Surface 尺寸 <=32768 且 scale <=64，因此这些转换可安全存入 int。
    // 极小的分数缩放仍至少需要一个后备像素。
    int width = (int)fmax(1, round(s->width * scale));
    int height = (int)fmax(1, round(s->height * scale));
    if (width != p->width || height != p->height || !p->buffer) {
        if (!allocate_pixels(s, p, width, height)) return;
    }
    if (!paint_pixels(s, p)) return;
    wl_surface_set_buffer_scale(s->surface, s->viewport ? 1 : s->active->scale);
    if (s->viewport) wl_proxy_marshal(s->viewport, 2, s->width, s->height);
    wl_surface_attach(s->surface, p->buffer, 0, 0);
    wl_surface_damage(s->surface, 0, 0, s->width, s->height);
    s->frame = wl_surface_frame(s->surface);
    if (!s->frame) { fail(s, "cannot allocate frame callback"); return; }
    wl_callback_add_listener(s->frame, &frame_listener, s);
    p->busy = true; s->dirty = false;
    wl_surface_commit(s->surface);
    if (!s->ready_sent) {
        s->ready_sent = true;
        arcanaWaylandReady(s->ready_handle);
    }
}
static void cleanup_native(struct arcana_wayland *s) {
    if (!s->display) return;
    destroy_surface(s);
    for (int i = 0; i < 2; ++i) free_pixels(&s->pixels[i]);
    while (s->outputs) { struct output *next = s->outputs->next; free_output(s->outputs); s->outputs = next; }
    destroy_extension(&s->fractional_manager, 0);
    destroy_extension(&s->viewporter, 0);
    destroy_extension(&s->xdg_manager, 0);
    if (s->shell) {
        if (wl_proxy_get_version(s->shell) >= 3) wl_proxy_marshal(s->shell, 1);
        wl_proxy_destroy(s->shell); s->shell = NULL;
    }
    if (s->shm) wl_shm_destroy(s->shm);
    if (s->compositor) wl_compositor_destroy(s->compositor);
    if (s->registry) wl_registry_destroy(s->registry);
    // 关闭时绝不执行 roundtrip；已死亡的合成器不能阻塞退出。
    wl_display_flush(s->display);
    wl_display_disconnect(s->display); s->display = NULL;
}

// 非交互式桌面 overlay 必须有 layer-shell，不能仅有 Wayland 连接。用户测试在
// Debian 13 ARM64 Parallels 虚拟机的完整 KWin 6.3.6 和 niri 26.04 会话中通过；
// 嵌套 KWin 渲染了中文文本和时钟；niri 检查确认 overlay-layer 放置及 keyboard
// 模式为 none。嵌套/容器缩放和工作区检查不能证明物理机行为。
// GNOME 48.7/Mutter 能成功启动 Wayland，但不公布此协议：这不是缺少软件包或
// 显示管理器配置问题。GNOME 支持需要单独的 Shell 集成，而不是普通窗口/X11 回退。
const char *arcana_wayland_run(struct arcana_wayland *s) {
    if (consume(s)) return NULL;
    if (s->error[0]) return s->error;
    if (!getenv("WAYLAND_DISPLAY") && !getenv("WAYLAND_SOCKET")) {
        fail(s, "no native Wayland session (WAYLAND_DISPLAY/WAYLAND_SOCKET missing); X11 is not supported");
        return s->error;
    }
    s->display = wl_display_connect(NULL);
    if (!s->display) { fail(s, "cannot connect to native Wayland display: %s (no X11 fallback)", strerror(errno)); return s->error; }
    s->registry = wl_display_get_registry(s->display);
    if (!s->registry) { fail(s, "cannot allocate Wayland registry"); goto done; }
    wl_registry_add_listener(s->registry, &registry_listener, s);
    // Registry、output 属性和 xdg-output 属性可能在不同的协议轮次中到达。
    // 所有同步等待都可响应取消。
    for (int i = 0; i < 3; ++i) if (synchronize(s)) goto done;
    if (!s->shell) { fail(s, "Wayland compositor lacks required zwlr_layer_shell_v1; GNOME/Mutter needs separate Shell integration; no normal-window or X11 fallback"); goto done; }
    if (!s->compositor || !s->shm) { fail(s, "compositor lacks wl_compositor or wl_shm"); goto done; }
    if (!s->argb) { fail(s, "compositor does not advertise ARGB8888 shared-memory buffers"); goto done; }
    for (;;) {
        if (consume(s) || s->error[0]) break;
        if (s->closed && synchronize(s)) break;
        // 输出公告先于 bind 后的名称/尺寸事件。按需等待协议栅栏，
        // 不把尚未收齐元数据的新显示器误判为不存在，也不做定时轮询。
        if (s->settle_outputs) {
            s->settle_outputs = false;
            if (synchronize(s)) break;
            if (s->settle_outputs) continue;
        }
        reconcile_output(s);
        if (s->error[0]) break;
        draw(s);
        if (s->error[0] || pump(s, -1)) break;
    }
done:
    cleanup_native(s);
    return s->error[0] ? s->error : NULL;
}
void arcana_wayland_free(struct arcana_wayland *s) {
    if (!s) return;
    close(s->wake[0]); close(s->wake[1]);
    pthread_mutex_destroy(&s->mutex);
    free_config(s->pending); free_config(s->config); free(s->wanted); free(s);
}
