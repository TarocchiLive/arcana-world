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

// Each output owns two bounded pixel stores and independent frame throttling.
#define MAX_BUFFER_BYTES (64u * 1024u * 1024u)
struct arcana_wayland;
struct output;
struct pixels
{
    struct wl_buffer *buffer;
    void *data;
    size_t size;
    int width, height, stride;
    bool busy;
};
struct view
{
    struct arcana_wayland *state;
    struct output *active;
    int width, height, effective_left, effective_top, effective_width, effective_height;
    struct wl_surface *surface;
    struct wl_proxy *layer, *viewport, *fractional;
    struct wl_callback *frame;
    struct pixels pixels[2];
    uint32_t fractional_scale;
    bool configured, dirty, closed;
};
struct output
{
    struct output *next;
    struct arcana_wayland *state;
    struct wl_output *proxy;
    struct wl_proxy *xdg;
    uint32_t id;
    int scale, mode_width, mode_height, transform, logical_width, logical_height;
    char *name, *description;
    bool ready;
    struct view view;
};
struct arcana_wayland
{
    // Only the mailbox and stop flag are shared with the Go producer.
    pthread_mutex_t mutex;
    int wake[2];
    bool stop;
    struct arcana_wayland_config *pending;
    // All remaining state belongs to the Wayland dispatch thread.
    struct arcana_wayland_config *config;
    char *wanted;
    char error[384];
    uintptr_t ready_handle, list_handle;
    bool ready_sent, updated, settle_outputs, argb, listing;
    struct wl_display *display;
    struct wl_registry *registry;
    struct wl_compositor *compositor;
    struct wl_shm *shm;
    struct wl_proxy *shell, *viewporter, *fractional_manager, *xdg_manager;
    struct output *outputs;
};
static void fail(struct arcana_wayland *s, const char *format, ...)
{
    if (s->error[0])
        return;
    va_list args;
    va_start(args, format);
    vsnprintf(s->error, sizeof(s->error), format, args);
    va_end(args);
}
static void destroy_extension(struct wl_proxy **proxy, uint32_t opcode)
{
    if (!*proxy)
        return;
    wl_proxy_marshal(*proxy, opcode);
    wl_proxy_destroy(*proxy);
    *proxy = NULL;
}

// 跨线程邮箱只保留最新快照。
static void wake(struct arcana_wayland *s)
{
    char c = 1;
    // EAGAIN 表示唤醒已在队列中。本线程不调用 Wayland。
    while (write(s->wake[1], &c, 1) < 0 && errno == EINTR)
    {
    }
}
static void free_config(struct arcana_wayland_config *config)
{
    if (!config)
        return;
    free(config->text);
    free(config->output);
    free(config->family);
    free(config->displays);
    free(config->text_runs);
    free(config);
}
static struct arcana_wayland_config *copy_config(const struct arcana_wayland_config *value)
{
    struct arcana_wayland_config *copy = malloc(sizeof(*copy));
    if (!copy)
        return NULL;
    *copy = *value;
    copy->text = strdup(value->text);
    copy->output = strdup(value->output);
    copy->family = strdup(value->family);
    copy->displays = value->displays_size ? malloc(value->displays_size) : NULL;
    copy->text_runs = value->text_run_count ? calloc(value->text_run_count, sizeof(*copy->text_runs)) : NULL;
    if (copy->text_runs)
        memcpy(copy->text_runs, value->text_runs, value->text_run_count * sizeof(*copy->text_runs));
    if (copy->displays)
        memcpy(copy->displays, value->displays, value->displays_size);
    if (!copy->text || !copy->output || !copy->family || (value->displays_size && !copy->displays) ||
        (value->text_run_count && !copy->text_runs))
    {
        free_config(copy);
        return NULL;
    }
    return copy;
}
static bool equal_paint_config(const struct arcana_wayland_config *a, const struct arcana_wayland_config *b)
{
    if (a->text_rgb != b->text_rgb || a->background_rgb != b->background_rgb ||
        a->text_run_count != b->text_run_count)
        return false;
    for (size_t i = 0; i < a->text_run_count; ++i)
        if (a->text_runs[i].start != b->text_runs[i].start ||
            a->text_runs[i].end != b->text_runs[i].end || a->text_runs[i].rgb != b->text_runs[i].rgb)
            return false;
    return !strcmp(a->text, b->text) && !strcmp(a->family, b->family) &&
           a->weight == b->weight && a->italic == b->italic && a->outline == b->outline &&
           a->padding_top == b->padding_top && a->padding_right == b->padding_right &&
           a->padding_bottom == b->padding_bottom && a->padding_left == b->padding_left &&
           a->font_size == b->font_size && a->text_alpha == b->text_alpha && a->background_alpha == b->background_alpha;
}
static bool equal_config(const struct arcana_wayland_config *a, const struct arcana_wayland_config *b)
{
    return equal_paint_config(a, b) && !strcmp(a->output, b->output) &&
           a->explicit_displays == b->explicit_displays && a->displays_size == b->displays_size &&
           (!a->displays_size || !memcmp(a->displays, b->displays, a->displays_size)) &&
           a->anchor == b->anchor && a->x == b->x && a->y == b->y &&
           a->width == b->width && a->height == b->height;
}
struct arcana_wayland *arcana_wayland_new(const struct arcana_wayland_config *config, uintptr_t ready_handle)
{
    struct arcana_wayland *s = calloc(1, sizeof(*s));
    if (!s)
        return NULL;
    if (pthread_mutex_init(&s->mutex, NULL))
    {
        free(s);
        return NULL;
    }
    if (pipe2(s->wake, O_CLOEXEC | O_NONBLOCK))
    {
        pthread_mutex_destroy(&s->mutex);
        free(s);
        return NULL;
    }
    s->config = copy_config(config);
    s->wanted = strdup(config->output);
    if (!s->config || !s->wanted)
    {
        arcana_wayland_free(s);
        return NULL;
    }
    s->ready_handle = ready_handle;
    return s;
}
int arcana_wayland_update(struct arcana_wayland *s, const struct arcana_wayland_config *config)
{
    struct arcana_wayland_config *copy = copy_config(config);
    if (!copy)
        return 0;
    pthread_mutex_lock(&s->mutex);
    bool notify = !s->pending;
    free_config(s->pending);
    s->pending = copy;
    pthread_mutex_unlock(&s->mutex);
    if (notify)
        wake(s);
    return 1;
}
void arcana_wayland_stop(struct arcana_wayland *s)
{
    pthread_mutex_lock(&s->mutex);
    s->stop = true;
    pthread_mutex_unlock(&s->mutex);
    wake(s);
}
static bool consume(struct arcana_wayland *s)
{
    char bytes[128];
    // 有界读取一次即可：如果生产者抢先写入，管道仍保持可读。
    (void)read(s->wake[0], bytes, sizeof(bytes));
    pthread_mutex_lock(&s->mutex);
    bool stop = s->stop;
    struct arcana_wayland_config *next = s->pending;
    s->pending = NULL;
    pthread_mutex_unlock(&s->mutex);
    if (next && !equal_config(next, s->config))
    {
        if (strcmp(next->output, s->config->output))
        {
            char *wanted = strdup(next->output);
            if (!wanted)
                fail(s, "cannot allocate output selector");
            else
            {
                free(s->wanted);
                s->wanted = wanted;
                s->settle_outputs = true;
            }
        }
        for (struct output *o = s->outputs; o; o = o->next)
            o->view.dirty = true;
        free_config(s->config);
        s->config = next;
        s->updated = true;
    }
    else
        free_config(next);
    return stop;
}

// 显示 I/O：调用方传入失败操作捕获的错误。
static void display_error(struct arcana_wayland *s, int io_error)
{
    int code = wl_display_get_error(s->display);
    if (code == EPROTO)
    {
        const struct wl_interface *interface = NULL;
        uint32_t id = 0;
        uint32_t protocol = wl_display_get_protocol_error(s->display, &interface, &id);
        fail(s, "protocol error %u on %s@%u", protocol, interface ? interface->name : "unknown", id);
    }
    else
        fail(s, "compositor disconnected or I/O failed: %s", strerror(code ? code : io_error));
}
// 事件返回 0，取消返回 1，错误返回 -1。prepare/read 配对保持分发单线程化，
// 并使取消即使在启动阶段也能中断。
static int pump(struct arcana_wayland *s, int timeout)
{
    if (consume(s))
        return 1;
    if (s->updated)
    {
        s->updated = false;
        return 0;
    }
    if (s->error[0])
        return -1;
    if (wl_display_prepare_read(s->display) != 0)
    {
        if (wl_display_dispatch_pending(s->display) < 0)
        {
            display_error(s, errno);
            return -1;
        }
        if (consume(s))
            return 1;
        return s->error[0] ? -1 : 0;
    }
    int flush = wl_display_flush(s->display);
    if (flush < 0 && errno != EAGAIN)
    {
        int code = errno;
        wl_display_cancel_read(s->display);
        display_error(s, code);
        return -1;
    }
    struct pollfd fds[2] = {
        {wl_display_get_fd(s->display), POLLIN | (flush < 0 ? POLLOUT : 0), 0},
        {s->wake[0], POLLIN, 0}};
    int result = poll(fds, 2, timeout);
    if (result < 0)
    {
        int code = errno;
        wl_display_cancel_read(s->display);
        if (code == EINTR)
            return 0;
        fail(s, "poll failed: %s", strerror(code));
        return -1;
    }
    if (fds[0].revents & POLLIN)
    {
        if (wl_display_read_events(s->display) < 0)
        {
            display_error(s, errno);
            return -1;
        }
    }
    else
        wl_display_cancel_read(s->display);
    // poll 通过 revents 而非 errno 报告失败。prepare_read/flush 可能留下
    // EAGAIN；报告该错误会掩盖实际的合成器挂断。
    if (fds[0].revents & (POLLERR | POLLHUP | POLLNVAL))
    {
        display_error(s, fds[0].revents & POLLNVAL ? EBADF : EPIPE);
        return -1;
    }
    if (wl_display_dispatch_pending(s->display) < 0)
    {
        display_error(s, errno);
        return -1;
    }
    if (consume(s))
        return 1;
    return s->error[0] ? -1 : 0;
}
struct sync_state
{
    bool done;
};
static void sync_done(void *data, struct wl_callback *callback, uint32_t serial)
{
    (void)callback;
    (void)serial;
    ((struct sync_state *)data)->done = true;
}
static const struct wl_callback_listener sync_listener = {sync_done};
static int synchronize(struct arcana_wayland *s)
{
    struct sync_state sync = {false};
    struct wl_callback *callback = wl_display_sync(s->display);
    if (!callback)
    {
        fail(s, "cannot allocate display sync");
        return -1;
    }
    wl_callback_add_listener(callback, &sync_listener, &sync);
    struct timespec start, now;
    clock_gettime(CLOCK_MONOTONIC, &start);
    int result = 0;
    while (!sync.done)
    {
        clock_gettime(CLOCK_MONOTONIC, &now);
        int64_t elapsed = (int64_t)(now.tv_sec - start.tv_sec) * 1000 + (now.tv_nsec - start.tv_nsec) / 1000000;
        if (elapsed >= 5000)
        {
            fail(s, "compositor initialization timed out");
            result = -1;
            break;
        }
        result = pump(s, (int)(5000 - elapsed));
        if (result || sync.done)
            break;
    }
    wl_callback_destroy(callback);
    return result;
}

// Output callbacks update only their own surface geometry and scale.
static void output_geometry(void *data, struct wl_output *p, int32_t x, int32_t y,
                            int32_t pw, int32_t ph, int32_t sub, const char *make, const char *model, int32_t transform)
{
    (void)p;
    (void)x;
    (void)y;
    (void)pw;
    (void)ph;
    (void)sub;
    // Connector identity comes from output.name; make/model are descriptive only.
    struct output *o = data;
    if (!o->description && (make || model))
    {
        char *description;
        if (asprintf(&description, "%s%s%s", make ? make : "", make && model ? " " : "", model ? model : "") < 0)
            fail(o->state, "cannot allocate output description");
        else
            o->description = description;
    }
    o->transform = transform;
    o->view.dirty = true;
}
static void output_mode(void *data, struct wl_output *p, uint32_t flags, int32_t w, int32_t h, int32_t refresh)
{
    (void)p;
    (void)refresh;
    if (!(flags & WL_OUTPUT_MODE_CURRENT))
        return;
    struct output *o = data;
    if (w <= 0 || h <= 0)
    {
        fail(o->state, "invalid output mode dimensions");
        return;
    }
    o->mode_width = w;
    o->mode_height = h;
    o->view.dirty = true;
}
static void output_done(void *data, struct wl_output *p)
{
    (void)p;
    ((struct output *)data)->ready = true;
}
static void output_scale(void *data, struct wl_output *p, int32_t scale)
{
    (void)p;
    struct output *o = data;
    if (scale < 1 || scale > 64)
    {
        fail(o->state, "invalid or excessive output scale: %d", scale);
        return;
    }
    o->scale = scale;
    o->view.dirty = true;
}
static void set_output_name(struct output *o, const char *name)
{
    char *copy = strdup(name);
    if (!copy)
    {
        fail(o->state, "cannot allocate output name");
        return;
    }
    free(o->name);
    o->name = copy;
}
static void output_name(void *data, struct wl_output *p, const char *name)
{
    (void)p;
    set_output_name(data, name);
}
static void output_description(void *data, struct wl_output *p, const char *description)
{
    (void)p;
    struct output *o = data;
    char *copy = strdup(description);
    if (!copy)
    {
        fail(o->state, "cannot allocate output description");
        return;
    }
    free(o->description);
    o->description = copy;
}
static const struct wl_output_listener output_listener = {
    output_geometry, output_mode, output_done, output_scale, output_name, output_description};
static void xdg_position(void *data, struct wl_proxy *p, int32_t x, int32_t y)
{
    (void)data;
    (void)p;
    (void)x;
    (void)y;
}
static void xdg_size(void *data, struct wl_proxy *p, int32_t w, int32_t h)
{
    (void)p;
    struct output *o = data;
    if (w <= 0 || h <= 0)
    {
        fail(o->state, "invalid logical output dimensions");
        return;
    }
    o->logical_width = w;
    o->logical_height = h;
    o->view.dirty = true;
}
static void xdg_done(void *data, struct wl_proxy *p)
{
    (void)p;
    ((struct output *)data)->ready = true;
}
static void xdg_name(void *data, struct wl_proxy *p, const char *name)
{
    (void)p;
    set_output_name(data, name);
}
static void xdg_description(void *data, struct wl_proxy *p, const char *description)
{
    output_description(data, NULL, description);
    (void)p;
}
static void (*const xdg_listener[])(void) = {
    (void (*)(void))xdg_position, (void (*)(void))xdg_size, (void (*)(void))xdg_done,
    (void (*)(void))xdg_name, (void (*)(void))xdg_description};
static void attach_xdg(struct arcana_wayland *s, struct output *o)
{
    if (!s->xdg_manager || o->xdg)
        return;
    o->xdg = (struct wl_proxy *)wl_proxy_marshal_constructor_versioned(s->xdg_manager, 1,
                                                                       &zxdg_output_v1_interface, wl_proxy_get_version(s->xdg_manager), NULL, o->proxy);
    if (!o->xdg)
    {
        fail(s, "cannot allocate xdg-output");
        return;
    }
    wl_proxy_add_listener(o->xdg, (void (**)(void))xdg_listener, o);
}
static void shm_format(void *data, struct wl_shm *shm, uint32_t format)
{
    (void)shm;
    if (format == WL_SHM_FORMAT_ARGB8888)
        ((struct arcana_wayland *)data)->argb = true;
}
static const struct wl_shm_listener shm_listener = {shm_format};
static void registry_global(void *data, struct wl_registry *registry, uint32_t id, const char *name, uint32_t version)
{
    struct arcana_wayland *s = data;
    if (!s->listing && !strcmp(name, "wl_compositor") && !s->compositor)
    {
        if (version < 3)
        {
            fail(s, "wl_compositor v3 or newer is required");
            return;
        }
        s->compositor = wl_registry_bind(registry, id, &wl_compositor_interface, version < 4 ? version : 4);
    }
    else if (!s->listing && !strcmp(name, "wl_shm") && !s->shm)
    {
        s->shm = wl_registry_bind(registry, id, &wl_shm_interface, 1);
        if (!s->shm)
        {
            fail(s, "cannot bind shared-memory interface");
            return;
        }
        wl_shm_add_listener(s->shm, &shm_listener, s);
    }
    else if (!s->listing && !strcmp(name, "zwlr_layer_shell_v1") && !s->shell)
    {
        s->shell = wl_registry_bind(registry, id, &zwlr_layer_shell_v1_interface, version < 4 ? version : 4);
    }
    else if (!s->listing && !strcmp(name, "wp_viewporter") && !s->viewporter)
    {
        s->viewporter = wl_registry_bind(registry, id, &wp_viewporter_interface, 1);
    }
    else if (!s->listing && !strcmp(name, "wp_fractional_scale_manager_v1") && !s->fractional_manager)
    {
        s->fractional_manager = wl_registry_bind(registry, id, &wp_fractional_scale_manager_v1_interface, 1);
    }
    else if (!strcmp(name, "zxdg_output_manager_v1") && !s->xdg_manager && version >= 2)
    {
        s->xdg_manager = wl_registry_bind(registry, id, &zxdg_output_manager_v1_interface, version < 3 ? version : 3);
        for (struct output *o = s->outputs; o; o = o->next)
            attach_xdg(s, o);
        s->settle_outputs = true;
    }
    else if (!strcmp(name, "wl_output"))
    {
        if (version < 2)
        {
            fail(s, "wl_output v2 or newer is required");
            return;
        }
        struct output *o = calloc(1, sizeof(*o));
        if (!o)
        {
            fail(s, "cannot allocate output");
            return;
        }
        o->state = s;
    o->view.state = s;
    o->view.active = o;
    o->id = id;
        o->scale = 1;
        o->proxy = wl_registry_bind(registry, id, &wl_output_interface, version < 4 ? version : 4);
        if (!o->proxy)
        {
            free(o);
            fail(s, "cannot bind output");
            return;
        }
        // 保留 registry 顺序，以确定性地选择默认 output/回退 output。
        struct output **tail = &s->outputs;
        while (*tail)
            tail = &(*tail)->next;
        *tail = o;
        wl_output_add_listener(o->proxy, &output_listener, o);
        attach_xdg(s, o);
        s->settle_outputs = true;
    }
}
static void destroy_surface(struct view *s);
static void free_pixels(struct pixels *p);
static void free_output(struct output *o)
{
    destroy_surface(&o->view);
    for (int i = 0; i < 2; ++i)
        free_pixels(&o->view.pixels[i]);
    destroy_extension(&o->xdg, 0);
    if (wl_output_get_version(o->proxy) >= 3)
        wl_output_release(o->proxy);
    else
        wl_output_destroy(o->proxy);
    free(o->name);
    free(o->description);
    free(o);
}
static void registry_remove(void *data, struct wl_registry *registry, uint32_t id)
{
    (void)registry;
    struct arcana_wayland *s = data;
    struct output **link = &s->outputs;
    while (*link)
    {
        struct output *o = *link;
        if (o->id == id)
        {
            s->settle_outputs = true;
            *link = o->next;
            free_output(o);
            return;
        }
        link = &o->next;
    }
}
static const struct wl_registry_listener registry_listener = {registry_global, registry_remove};
static void layer_configure(void *data, struct wl_proxy *layer, uint32_t serial, uint32_t width, uint32_t height)
{
    struct view *s = data;
    wl_proxy_marshal(layer, 6, serial);
    if (width > 32768 || height > 32768)
    {
        fail(s->state, "compositor configured an excessive overlay size");
        return;
    }
    int configured_width = width ? (int)width : s->effective_width;
    int configured_height = height ? (int)height : s->effective_height;
    if (!s->configured || s->width != configured_width || s->height != configured_height)
        s->dirty = true;
    s->width = configured_width;
    s->height = configured_height;
    s->configured = true;
}
static void layer_closed(void *data, struct wl_proxy *layer)
{
    (void)layer;
    ((struct view *)data)->closed = true;
}
static void (*const layer_listener[])(void) = {(void (*)(void))layer_configure, (void (*)(void))layer_closed};
static void preferred_scale(void *data, struct wl_proxy *proxy, uint32_t scale)
{
    (void)proxy;
    struct view *s = data;
    if (!scale || scale > 7680)
    {
        fail(s->state, "invalid or excessive fractional scale: %u", scale);
        return;
    }
    s->fractional_scale = scale;
    s->dirty = true;
}
static void (*const fractional_listener[])(void) = {(void (*)(void))preferred_scale};

static void destroy_surface(struct view *s)
{
    if (s->frame)
    {
        wl_callback_destroy(s->frame);
        s->frame = NULL;
    }
    destroy_extension(&s->fractional, 0);
    destroy_extension(&s->viewport, 0);
    destroy_extension(&s->layer, 7);
    if (s->surface)
    {
        wl_surface_destroy(s->surface);
        s->surface = NULL;
    }
    s->configured = false;
    s->closed = false;
    s->fractional_scale = 0;
}
static void surface_output_changed(void *data, struct wl_surface *surface, struct wl_output *output)
{
    (void)surface;
    (void)output;
    ((struct view *)data)->dirty = true;
}
static const struct wl_surface_listener surface_listener = {
    .enter = surface_output_changed, .leave = surface_output_changed};
static bool output_bounds(struct view *s, int *width, int *height)
{
    struct output *o = s->active;
    // xdg-output 的逻辑尺寸已包含旋转和分数缩放。
    // 核心 wl_output 模式是物理像素，需应用这两项变换。
    if (o->logical_width > 0 && o->logical_height > 0)
    {
        *width = o->logical_width;
        *height = o->logical_height;
    }
    else
    {
        bool swapped = o->transform == WL_OUTPUT_TRANSFORM_90 ||
                       o->transform == WL_OUTPUT_TRANSFORM_270 ||
                       o->transform == WL_OUTPUT_TRANSFORM_FLIPPED_90 ||
                       o->transform == WL_OUTPUT_TRANSFORM_FLIPPED_270;
        double scale = s->viewport && s->fractional_scale ? s->fractional_scale / 120.0 : o->scale;
        double w = round((swapped ? o->mode_height : o->mode_width) / scale);
        double h = round((swapped ? o->mode_width : o->mode_height) / scale);
        if (w > INT_MAX || h > INT_MAX)
            return false;
        *width = (int)w;
        *height = (int)h;
    }
    return *width > 0 && *height > 0;
}
static double anchor_offset(int axis, double offset, double available)
{
    if (axis == 1)
        offset = available / 2 + offset;
    else if (axis == 2)
        offset = available - offset;
    return fmax(0, fmin(offset, available));
}
static bool apply_geometry(struct view *s, bool initial)
{
    int output_width, output_height;
    if (!output_bounds(s, &output_width, &output_height))
    {
        fail(s->state, "selected output has no usable full-output geometry");
        return false;
    }
    const struct arcana_wayland_config *c = s->state->config;
    double bounded_width = fmin(c->width, output_width);
    double bounded_height = fmin(c->height, output_height);
    if (bounded_width > 32768 || bounded_height > 32768)
    {
        fail(s->state, "output-clamped overlay size exceeds native rendering limits");
        return false;
    }
    int width = (int)fmax(1, round(bounded_width)), height = (int)fmax(1, round(bounded_height));
    int left = (int)round(anchor_offset(c->anchor % 3, c->x, output_width - width));
    int top = (int)round(anchor_offset(c->anchor / 3, c->y, output_height - height));
    if (!initial && width == s->effective_width && height == s->effective_height &&
        left == s->effective_left && top == s->effective_top)
        return false;
    bool resized = initial || width != s->effective_width || height != s->effective_height;
    s->effective_left = left;
    s->effective_top = top;
    s->effective_width = width;
    s->effective_height = height;
    // Resolve all nine anchors against the full output, not its usable work area.
    wl_proxy_marshal(s->layer, 0, (uint32_t)width, (uint32_t)height);
    wl_proxy_marshal(s->layer, 3, top, 0, 0, left);
    if (resized)
    {
        s->configured = false;
        s->dirty = true;
    }
    return true;
}
static void create_surface(struct view *s, struct output *output)
{
    s->active = output;
    s->surface = wl_compositor_create_surface(s->state->compositor);
    if (!s->surface)
    {
        fail(s->state, "cannot allocate surface");
        return;
    }
    wl_surface_add_listener(s->surface, &surface_listener, s);
    s->layer = (struct wl_proxy *)wl_proxy_marshal_constructor_versioned(s->state->shell, 0,
                                                                         &zwlr_layer_surface_v1_interface, wl_proxy_get_version(s->state->shell), NULL,
                                                                         s->surface, output->proxy, 3u, "arcana-world-overlay");
    if (!s->layer)
    {
        fail(s->state, "cannot allocate layer surface");
        return;
    }
    wl_proxy_add_listener(s->layer, (void (**)(void))layer_listener, s);
    wl_proxy_marshal(s->layer, 1, 1u | 4u); // Top and left; margins carry the resolved position.
    wl_proxy_marshal(s->layer, 2, -1);      // 使用完整输出区域。
    wl_proxy_marshal(s->layer, 4, 0u);      // 禁用键盘交互。
    apply_geometry(s, true);
    if (s->state->error[0])
        return;
    struct wl_region *empty = wl_compositor_create_region(s->state->compositor);
    if (!empty)
    {
        fail(s->state, "cannot allocate empty input region");
        return;
    }
    wl_surface_set_input_region(s->surface, empty); // NULL 会接受输入。
    wl_region_destroy(empty);
    if (s->state->viewporter && s->state->fractional_manager)
    {
        s->viewport = (struct wl_proxy *)wl_proxy_marshal_constructor_versioned(s->state->viewporter, 1,
                                                                                &wp_viewport_interface, 1, NULL, s->surface);
        s->fractional = (struct wl_proxy *)wl_proxy_marshal_constructor_versioned(s->state->fractional_manager, 1,
                                                                                  &wp_fractional_scale_v1_interface, 1, NULL, s->surface);
        if (!s->viewport || !s->fractional)
        {
            fail(s->state, "cannot allocate fractional scaling objects");
            return;
        }
        wl_proxy_add_listener(s->fractional, (void (**)(void))fractional_listener, s);
    }
    s->dirty = true;
    wl_surface_commit(s->surface); // 初始空提交，在确认前不带 buffer。
}
static bool explicitly_selected(const struct arcana_wayland_config *c, const char *name)
{
    if (!name)
        return false;
    for (size_t offset = 0; offset < c->displays_size; offset += strlen(c->displays + offset) + 1)
        if (!strcmp(name, c->displays + offset))
            return true;
    return false;
}
static void reconcile_output(struct arcana_wayland *s)
{
    struct output *legacy = NULL, *fallback = NULL;
    for (struct output *o = s->outputs; o; o = o->next)
    {
        if (!o->ready)
            continue;
        if (!o->name || !*o->name)
        {
            fail(s, "stable output names require wl_output v4 or xdg-output v2");
            return;
        }
        if (!fallback)
            fallback = o;
        if (!strcmp(o->name, s->wanted))
            legacy = o;
    }
    if (!s->config->explicit_displays && !s->ready_sent && !legacy && *s->config->output)
    {
        fail(s, "output '%s' was not found (requires wl_output v4 or xdg-output v2 names)", s->config->output);
        return;
    }
    if (!s->config->explicit_displays && !s->ready_sent && !fallback)
    {
        fail(s, "no Wayland output is available");
        return;
    }
    if (!s->config->explicit_displays && !*s->wanted && fallback)
    {
        char *name = strdup(fallback->name);
        if (!name)
        {
            fail(s, "cannot capture default output identity");
            return;
        }
        free(s->wanted);
        s->wanted = name;
        legacy = fallback;
    }
    // Only the legacy automatic selector may fall back to another output.
    if (!s->config->explicit_displays && !*s->config->output && !legacy)
        legacy = fallback;
    for (struct output *o = s->outputs; o; o = o->next)
    {
        struct view *v = &o->view;
        bool selected = o->ready && (s->config->explicit_displays ? explicitly_selected(s->config, o->name) : o == legacy);
        if (!selected)
        {
            if (v->surface)
                destroy_surface(v);
            continue;
        }
        if (v->closed)
        {
            fail(s, "compositor closed the overlay layer surface on %s", o->name);
            return;
        }
        if (!v->surface)
            create_surface(v, o);
        else if (apply_geometry(v, false))
            wl_surface_commit(v->surface);
    }
}
static void buffer_release(void *data, struct wl_buffer *buffer)
{
    (void)buffer;
    ((struct pixels *)data)->busy = false;
}

// 像素存储可比表面存活更久；仅在 wl_buffer.release 后复用。
static const struct wl_buffer_listener buffer_listener = {buffer_release};
static void free_pixels(struct pixels *p)
{
    if (p->buffer)
        wl_buffer_destroy(p->buffer);
    if (p->data)
        munmap(p->data, p->size);
    memset(p, 0, sizeof(*p));
}
static bool allocate_pixels(struct arcana_wayland *s, struct pixels *p, int width, int height)
{
    int stride = cairo_format_stride_for_width(CAIRO_FORMAT_ARGB32, width);
    if (stride <= 0 || height <= 0 || (size_t)height > MAX_BUFFER_BYTES / (size_t)stride)
    {
        fail(s, "scaled overlay backing store exceeds 64 MiB");
        return false;
    }
    free_pixels(p);
    p->width = width;
    p->height = height;
    p->stride = stride;
    p->size = (size_t)stride * height;
    int fd = memfd_create("arcana-world-overlay", MFD_CLOEXEC);
    if (fd < 0)
    {
        fail(s, "memfd_create failed: %s", strerror(errno));
        return false;
    }
    if (ftruncate(fd, (off_t)p->size))
    {
        fail(s, "cannot size pixel store: %s", strerror(errno));
        close(fd);
        return false;
    }
    p->data = mmap(NULL, p->size, PROT_READ | PROT_WRITE, MAP_SHARED, fd, 0);
    if (p->data == MAP_FAILED)
    {
        p->data = NULL;
        fail(s, "cannot map pixel store: %s", strerror(errno));
        close(fd);
        return false;
    }
    struct wl_shm_pool *pool = wl_shm_create_pool(s->shm, fd, (int)p->size);
    close(fd);
    if (!pool)
    {
        fail(s, "cannot allocate shared-memory pool");
        return false;
    }
    p->buffer = wl_shm_pool_create_buffer(pool, 0, width, height, stride, WL_SHM_FORMAT_ARGB8888);
    wl_shm_pool_destroy(pool);
    if (!p->buffer)
    {
        fail(s, "cannot allocate Wayland buffer");
        return false;
    }
    wl_buffer_add_listener(p->buffer, &buffer_listener, p);
    return true;
}
static void frame_done(void *data, struct wl_callback *callback, uint32_t time)
{
    (void)time;
    struct view *s = data;
    wl_callback_destroy(callback);
    s->frame = NULL;
}
static const struct wl_callback_listener frame_listener = {frame_done};

// 仅绘制到已释放的存储；呈现与帧节流在后续处理。
static bool paint_pixels(struct view *s, struct pixels *p)
{
    // Cairo ARGB32 是本机字节序的预乘格式，与 wl_shm ARGB8888 匹配。
    cairo_surface_t *image = cairo_image_surface_create_for_data(p->data, CAIRO_FORMAT_ARGB32, p->width, p->height, p->stride);
    cairo_t *cr = cairo_create(image);
    const struct arcana_wayland_config *c = s->state->config;
    cairo_scale(cr, (double)p->width / s->width, (double)p->height / s->height);
    cairo_set_operator(cr, CAIRO_OPERATOR_SOURCE);
    cairo_set_source_rgba(cr, ((c->background_rgb >> 16) & 255) / 255.0,
                          ((c->background_rgb >> 8) & 255) / 255.0,
                          (c->background_rgb & 255) / 255.0, c->background_alpha);
    cairo_paint(cr);
    cairo_set_operator(cr, CAIRO_OPERATOR_OVER);
    double left = fmin(c->padding_left, s->width), top = fmin(c->padding_top, s->height);
    double width = fmax(0, s->width - left - fmin(c->padding_right, s->width));
    double height = fmax(0, s->height - top - fmin(c->padding_bottom, s->height));
    // Pango 将 alpha=0 解释为未指定；完全透明时不提交文字绘制。
    guint16 text_alpha = (guint16)round(c->text_alpha * 65535);
    if (width <= 0 || height <= 0 || !*c->text || !text_alpha)
        goto done;
    size_t text_length = strlen(c->text);
    if (text_length > INT_MAX)
    {
        fail(s->state, "overlay text exceeds Pango's signed length limit");
        cairo_destroy(cr);
        cairo_surface_destroy(image);
        return false;
    }
    PangoLayout *layout = pango_cairo_create_layout(cr);
    PangoFontDescription *font = pango_font_description_new();
    pango_font_description_set_family(font, *c->family ? c->family : "sans");
    pango_font_description_set_absolute_size(font, c->font_size * PANGO_SCALE);
    pango_font_description_set_weight(font, (PangoWeight)c->weight);
    pango_font_description_set_style(font, c->italic ? PANGO_STYLE_ITALIC : PANGO_STYLE_NORMAL);
    pango_layout_set_font_description(layout, font);
    pango_layout_set_width(layout, (int)fmin(width * PANGO_SCALE, INT_MAX));
    pango_layout_set_wrap(layout, PANGO_WRAP_WORD_CHAR);
    pango_layout_set_text(layout, c->text, (int)text_length);
    // Pango 区间使用原始 UTF-8 字节偏移；完整布局保留换行、包裹和尾部裁切的颜色。
    PangoAttrList *attributes = pango_attr_list_new();
    for (size_t i = 0; i < c->text_run_count; ++i)
    {
        const struct arcana_wayland_text_run *run = &c->text_runs[i];
        PangoAttribute *foreground = pango_attr_foreground_new(
            ((run->rgb >> 16) & 255) * 257, ((run->rgb >> 8) & 255) * 257, (run->rgb & 255) * 257);
        foreground->start_index = (guint)run->start;
        foreground->end_index = (guint)run->end;
        pango_attr_list_insert(attributes, foreground);
    }
    // foreground 会替换 Cairo 源颜色；透明度必须作为独立属性应用于全部文本。
    PangoAttribute *alpha = pango_attr_foreground_alpha_new(text_alpha);
    pango_attr_list_insert(attributes, alpha);
    pango_layout_set_attributes(layout, attributes);
    pango_attr_list_unref(attributes);
    // 配置字体决定行高；先转 double 再相加，避免整数度量溢出。
    PangoContext *context = pango_layout_get_context(layout);
    PangoFontMetrics *metrics = pango_context_get_metrics(context, font, pango_context_get_language(context));
    double ascent = (double)pango_font_metrics_get_ascent(metrics) / PANGO_SCALE;
    double descent = (double)pango_font_metrics_get_descent(metrics) / PANGO_SCALE;
    double line_height = ceil(fmax(ascent + descent,
                                   (double)pango_font_metrics_get_height(metrics) / PANGO_SCALE));
    pango_font_metrics_unref(metrics);
    if (line_height > 0 && height >= line_height)
    {
        // 从完整整形布局选取尾部视觉行，避免累计全文高度；回退字形裁剪在本行槽内。
        GSList *lines = pango_layout_get_lines_readonly(layout);
        int line_count = pango_layout_get_line_count(layout);
        int visible = (int)fmin(floor(height / line_height), line_count);
        for (int skip = line_count - visible; skip > 0; --skip)
            lines = lines->next;
        cairo_set_source_rgba(cr, ((c->text_rgb >> 16) & 255) / 255.0,
                              ((c->text_rgb >> 8) & 255) / 255.0,
                              (c->text_rgb & 255) / 255.0, c->text_alpha);
        double baseline = ascent + (line_height - ascent - descent) / 2;
        for (int row = 0; row < visible; ++row, lines = lines->next)
        {
            PangoLayoutLine *line = lines->data;
            PangoRectangle logical;
            pango_layout_line_get_extents(line, NULL, &logical);
            double x = left;
            // Pango 的默认 auto-dir 会将 RTL 段落靠右放置。
            if (line->resolved_dir == PANGO_DIRECTION_RTL)
                x += (double)pango_layout_get_width(layout) / PANGO_SCALE -
                     (double)logical.width / PANGO_SCALE;
            double y = top + row * line_height;
            cairo_save(cr);
            cairo_rectangle(cr, left, y, width, line_height);
            cairo_clip(cr);
            if (c->outline)
            {
                cairo_save(cr);
                cairo_move_to(cr, x, y + baseline);
                pango_cairo_layout_line_path(cr, line);
                cairo_set_source_rgba(cr, 16.0 / 255, 19.0 / 255, 24.0 / 255, c->text_alpha);
                cairo_set_line_width(cr, 1.0);
                cairo_set_line_join(cr, CAIRO_LINE_JOIN_ROUND);
                cairo_stroke(cr);
                cairo_restore(cr);
            }
            cairo_move_to(cr, x, y + baseline);
            pango_cairo_show_layout_line(cr, line);
            cairo_restore(cr);
        }
    }
    pango_font_description_free(font);
    g_object_unref(layout);
done:
    cairo_surface_flush(image);
    cairo_status_t status = cairo_status(cr);
    if (status == CAIRO_STATUS_SUCCESS)
        status = cairo_surface_status(image);
    cairo_destroy(cr);
    cairo_surface_destroy(image);
    if (status != CAIRO_STATUS_SUCCESS)
    {
        fail(s->state, "Cairo drawing failed: %s", cairo_status_to_string(status));
        return false;
    }
    return true;
}

// 渲染按帧节流，并且还需等待已释放的像素存储。
static void draw(struct view *s)
{
    if (!s->dirty || !s->configured || s->frame || !s->active || s->state->error[0])
        return;
    struct pixels *p = NULL;
    for (int i = 0; i < 2; ++i)
        if (!s->pixels[i].busy)
        {
            p = &s->pixels[i];
            break;
        }
    if (!p)
        return;
    double scale = s->viewport && s->fractional_scale ? s->fractional_scale / 120.0 : s->active->scale;
    // Surface 尺寸 <=32768 且 scale <=64，因此这些转换可安全存入 int。
    // 极小的分数缩放仍至少需要一个后备像素。
    int width = (int)fmax(1, round(s->width * scale));
    int height = (int)fmax(1, round(s->height * scale));
    if (width != p->width || height != p->height || !p->buffer)
    {
        if (!allocate_pixels(s->state, p, width, height))
            return;
    }
    if (!paint_pixels(s, p))
        return;
    wl_surface_set_buffer_scale(s->surface, s->viewport ? 1 : s->active->scale);
    if (s->viewport)
        wl_proxy_marshal(s->viewport, 2, s->width, s->height);
    wl_surface_attach(s->surface, p->buffer, 0, 0);
    wl_surface_damage(s->surface, 0, 0, s->width, s->height);
    s->frame = wl_surface_frame(s->surface);
    if (!s->frame)
    {
        fail(s->state, "cannot allocate frame callback");
        return;
    }
    wl_callback_add_listener(s->frame, &frame_listener, s);
    p->busy = true;
    s->dirty = false;
    wl_surface_commit(s->surface);
}
static void cleanup_native(struct arcana_wayland *s)
{
    if (!s->display)
        return;
    // Output destruction also releases its surface and pixel stores.
    while (s->outputs)
    {
        struct output *next = s->outputs->next;
        free_output(s->outputs);
        s->outputs = next;
    }
    destroy_extension(&s->fractional_manager, 0);
    destroy_extension(&s->viewporter, 0);
    destroy_extension(&s->xdg_manager, 0);
    if (s->shell)
    {
        if (wl_proxy_get_version(s->shell) >= 3)
            wl_proxy_marshal(s->shell, 1);
        wl_proxy_destroy(s->shell);
        s->shell = NULL;
    }
    if (s->shm)
        wl_shm_destroy(s->shm);
    if (s->compositor)
        wl_compositor_destroy(s->compositor);
    if (s->registry)
        wl_registry_destroy(s->registry);
    // 关闭时绝不执行 roundtrip；已死亡的合成器不能阻塞退出。
    wl_display_flush(s->display);
    wl_display_disconnect(s->display);
    s->display = NULL;
}

// 桌面浮层依赖 layer-shell；缺少协议时明确报错，不回退到普通窗口或 X11。
const char *arcana_wayland_run(struct arcana_wayland *s)
{
    if (consume(s))
        return NULL;
    if (s->error[0])
        return s->error;
    if (!getenv("WAYLAND_DISPLAY") && !getenv("WAYLAND_SOCKET"))
    {
        fail(s, "no native Wayland session (WAYLAND_DISPLAY/WAYLAND_SOCKET missing); X11 is not supported");
        return s->error;
    }
    s->display = wl_display_connect(NULL);
    if (!s->display)
    {
        fail(s, "cannot connect to native Wayland display: %s (no X11 fallback)", strerror(errno));
        return s->error;
    }
    s->registry = wl_display_get_registry(s->display);
    if (!s->registry)
    {
        fail(s, "cannot allocate Wayland registry");
        goto done;
    }
    wl_registry_add_listener(s->registry, &registry_listener, s);
    // Registry、output 属性和 xdg-output 属性可能在不同的协议轮次中到达。
    // 所有同步等待都可响应取消。
    for (int i = 0; i < 3; ++i)
        if (synchronize(s))
            goto done;
    if (s->listing)
    {
        for (struct output *o = s->outputs; o; o = o->next)
        {
            if (!o->ready || !o->name || !*o->name)
            {
                fail(s, "stable output names require wl_output v4 or xdg-output v2");
                goto done;
            }
            arcanaWaylandDisplay(s->list_handle, o->name, o->description && *o->description ? o->description : o->name);
        }
        goto done;
    }
    if (!s->shell)
    {
        fail(s, "Wayland compositor lacks required zwlr_layer_shell_v1; GNOME/Mutter needs separate Shell integration; no normal-window or X11 fallback");
        goto done;
    }
    if (!s->compositor || !s->shm)
    {
        fail(s, "compositor lacks wl_compositor or wl_shm");
        goto done;
    }
    if (!s->argb)
    {
        fail(s, "compositor does not advertise ARGB8888 shared-memory buffers");
        goto done;
    }
    for (;;)
    {
        if (consume(s) || s->error[0])
            break;
        bool closed = false;
        for (struct output *o = s->outputs; o; o = o->next)
            closed = closed || o->view.closed;
        if (closed && synchronize(s))
            break;
        // 输出公告先于 bind 后的名称/尺寸事件。按需等待协议栅栏，
        // 不把尚未收齐元数据的新显示器误判为不存在，也不做定时轮询。
        if (s->settle_outputs)
        {
            s->settle_outputs = false;
            if (synchronize(s))
                break;
            if (s->settle_outputs)
                continue;
        }
        reconcile_output(s);
        if (s->error[0])
            break;
        bool ready = true;
        for (struct output *o = s->outputs; o; o = o->next)
        {
            draw(&o->view);
            if (o->view.surface && (!o->view.configured || o->view.dirty))
                ready = false;
        }
        if (ready && !s->ready_sent && !s->error[0])
        {
            s->ready_sent = true;
            arcanaWaylandReady(s->ready_handle);
        }
        if (s->error[0] || pump(s, -1))
            break;
    }
done:
    cleanup_native(s);
    return s->error[0] ? s->error : NULL;
}
const char *arcana_wayland_list(struct arcana_wayland *s, uintptr_t handle)
{
    s->listing = true;
    s->list_handle = handle;
    return arcana_wayland_run(s);
}
void arcana_wayland_free(struct arcana_wayland *s)
{
    if (!s)
        return;
    close(s->wake[0]);
    close(s->wake[1]);
    pthread_mutex_destroy(&s->mutex);
    free_config(s->pending);
    free_config(s->config);
    free(s->wanted);
    free(s);
}
