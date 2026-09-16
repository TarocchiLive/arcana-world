/* 已签入的 wire 描述符源自相邻的权威 XML。
 * 请求/事件顺序和签名（包括 since 版本及可为 NULL 的参数）与这些 XML 匹配。
 * 保留未使用的消息：其槽位定义操作码。
 * 描述符仅供 wayland_linux.c 私用；无需扫描器或导出符号。
 * 后端最多绑定此处所表示的版本。 */
#ifndef ARCANA_WAYLAND_PROTOCOLS_H
#define ARCANA_WAYLAND_PROTOCOLS_H
#include <wayland-client.h>
/*
    Copyright © 2017 Drew DeVault

    Permission to use, copy, modify, distribute, and sell this
    software and its documentation for any purpose is hereby granted
    without fee, provided that the above copyright notice appear in
    all copies and that both that copyright notice and this permission
    notice appear in supporting documentation, and that the name of
    the copyright holders not be used in advertising or publicity
    pertaining to distribution of the software without specific,
    written prior permission.  The copyright holders make no
    representations about the suitability of this software for any
    purpose.  It is provided "as is" without express or implied
    warranty.

    THE COPYRIGHT HOLDERS DISCLAIM ALL WARRANTIES WITH REGARD TO THIS
    SOFTWARE, INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY AND
    FITNESS, IN NO EVENT SHALL THE COPYRIGHT HOLDERS BE LIABLE FOR ANY
    SPECIAL, INDIRECT OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES
    WHATSOEVER RESULTING FROM LOSS OF USE, DATA OR PROFITS, WHETHER IN
    AN ACTION OF CONTRACT, NEGLIGENCE OR OTHER TORTIOUS ACTION,
    ARISING OUT OF OR IN CONNECTION WITH THE USE OR PERFORMANCE OF
    THIS SOFTWARE.
  */
/*
    Copyright © 2022 Kenny Levinsen

    Permission is hereby granted, free of charge, to any person obtaining a
    copy of this software and associated documentation files (the "Software"),
    to deal in the Software without restriction, including without limitation
    the rights to use, copy, modify, merge, publish, distribute, sublicense,
    and/or sell copies of the Software, and to permit persons to whom the
    Software is furnished to do so, subject to the following conditions:

    The above copyright notice and this permission notice (including the next
    paragraph) shall be included in all copies or substantial portions of the
    Software.

    THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
    IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
    FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.  IN NO EVENT SHALL
    THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
    LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
    FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
    DEALINGS IN THE SOFTWARE.
  */
/*
    Copyright © 2017 Red Hat Inc.

    Permission is hereby granted, free of charge, to any person obtaining a
    copy of this software and associated documentation files (the "Software"),
    to deal in the Software without restriction, including without limitation
    the rights to use, copy, modify, merge, publish, distribute, sublicense,
    and/or sell copies of the Software, and to permit persons to whom the
    Software is furnished to do so, subject to the following conditions:

    The above copyright notice and this permission notice (including the next
    paragraph) shall be included in all copies or substantial portions of the
    Software.

    THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
    IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
    FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.  IN NO EVENT SHALL
    THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
    LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
    FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
    DEALINGS IN THE SOFTWARE.
  */
/*
    Copyright © 2013-2016 Collabora, Ltd.

    Permission is hereby granted, free of charge, to any person obtaining a
    copy of this software and associated documentation files (the "Software"),
    to deal in the Software without restriction, including without limitation
    the rights to use, copy, modify, merge, publish, distribute, sublicense,
    and/or sell copies of the Software, and to permit persons to whom the
    Software is furnished to do so, subject to the following conditions:

    The above copyright notice and this permission notice (including the next
    paragraph) shall be included in all copies or substantial portions of the
    Software.

    THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
    IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
    FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT.  IN NO EVENT SHALL
    THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
    LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
    FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
    DEALINGS IN THE SOFTWARE.
  */
static const struct wl_interface zwlr_layer_shell_v1_interface;
static const struct wl_interface zwlr_layer_surface_v1_interface;
static const struct wl_interface wp_fractional_scale_manager_v1_interface;
static const struct wl_interface wp_fractional_scale_v1_interface;
static const struct wl_interface zxdg_output_manager_v1_interface;
static const struct wl_interface zxdg_output_v1_interface;
static const struct wl_interface wp_viewporter_interface;
static const struct wl_interface wp_viewport_interface;
/* 只有未使用的 layer-shell get_popup 签名需要此接口名称；
 * 此后端从不创建 popup，也不在 xdg_popup 上编组请求。 */
static const struct wl_interface xdg_popup_interface = {"xdg_popup", 1, 0, NULL, 0, NULL};
static const struct wl_interface *zwlr_layer_shell_v1_request_0_types[] = {&zwlr_layer_surface_v1_interface, &wl_surface_interface, &wl_output_interface, NULL, NULL};
static const struct wl_message zwlr_layer_shell_v1_requests[] = {{"get_layer_surface", "no?ous", zwlr_layer_shell_v1_request_0_types},
{"destroy", "3", NULL}};
static const struct wl_interface zwlr_layer_shell_v1_interface = {"zwlr_layer_shell_v1", 4, 2, zwlr_layer_shell_v1_requests, 0, NULL};
static const struct wl_interface *zwlr_layer_surface_v1_request_0_types[] = {NULL, NULL};
static const struct wl_interface *zwlr_layer_surface_v1_request_1_types[] = {NULL};
static const struct wl_interface *zwlr_layer_surface_v1_request_2_types[] = {NULL};
static const struct wl_interface *zwlr_layer_surface_v1_request_3_types[] = {NULL, NULL, NULL, NULL};
static const struct wl_interface *zwlr_layer_surface_v1_request_4_types[] = {NULL};
static const struct wl_interface *zwlr_layer_surface_v1_request_5_types[] = {&xdg_popup_interface};
static const struct wl_interface *zwlr_layer_surface_v1_request_6_types[] = {NULL};
static const struct wl_interface *zwlr_layer_surface_v1_request_8_types[] = {NULL};
static const struct wl_message zwlr_layer_surface_v1_requests[] = {{"set_size", "uu", zwlr_layer_surface_v1_request_0_types},
{"set_anchor", "u", zwlr_layer_surface_v1_request_1_types},
{"set_exclusive_zone", "i", zwlr_layer_surface_v1_request_2_types},
{"set_margin", "iiii", zwlr_layer_surface_v1_request_3_types},
{"set_keyboard_interactivity", "u", zwlr_layer_surface_v1_request_4_types},
{"get_popup", "o", zwlr_layer_surface_v1_request_5_types},
{"ack_configure", "u", zwlr_layer_surface_v1_request_6_types},
{"destroy", "", NULL},
{"set_layer", "2u", zwlr_layer_surface_v1_request_8_types}};
static const struct wl_interface *zwlr_layer_surface_v1_event_0_types[] = {NULL, NULL, NULL};
static const struct wl_message zwlr_layer_surface_v1_events[] = {{"configure", "uuu", zwlr_layer_surface_v1_event_0_types},
{"closed", "", NULL}};
static const struct wl_interface zwlr_layer_surface_v1_interface = {"zwlr_layer_surface_v1", 4, 9, zwlr_layer_surface_v1_requests, 2, zwlr_layer_surface_v1_events};
static const struct wl_interface *wp_fractional_scale_manager_v1_request_1_types[] = {&wp_fractional_scale_v1_interface, &wl_surface_interface};
static const struct wl_message wp_fractional_scale_manager_v1_requests[] = {{"destroy", "", NULL},
{"get_fractional_scale", "no", wp_fractional_scale_manager_v1_request_1_types}};
static const struct wl_interface wp_fractional_scale_manager_v1_interface = {"wp_fractional_scale_manager_v1", 1, 2, wp_fractional_scale_manager_v1_requests, 0, NULL};
static const struct wl_message wp_fractional_scale_v1_requests[] = {{"destroy", "", NULL}};
static const struct wl_interface *wp_fractional_scale_v1_event_0_types[] = {NULL};
static const struct wl_message wp_fractional_scale_v1_events[] = {{"preferred_scale", "u", wp_fractional_scale_v1_event_0_types}};
static const struct wl_interface wp_fractional_scale_v1_interface = {"wp_fractional_scale_v1", 1, 1, wp_fractional_scale_v1_requests, 1, wp_fractional_scale_v1_events};
static const struct wl_interface *zxdg_output_manager_v1_request_1_types[] = {&zxdg_output_v1_interface, &wl_output_interface};
static const struct wl_message zxdg_output_manager_v1_requests[] = {{"destroy", "", NULL},
{"get_xdg_output", "no", zxdg_output_manager_v1_request_1_types}};
static const struct wl_interface zxdg_output_manager_v1_interface = {"zxdg_output_manager_v1", 3, 2, zxdg_output_manager_v1_requests, 0, NULL};
static const struct wl_message zxdg_output_v1_requests[] = {{"destroy", "", NULL}};
static const struct wl_interface *zxdg_output_v1_event_0_types[] = {NULL, NULL};
static const struct wl_interface *zxdg_output_v1_event_1_types[] = {NULL, NULL};
static const struct wl_interface *zxdg_output_v1_event_3_types[] = {NULL};
static const struct wl_interface *zxdg_output_v1_event_4_types[] = {NULL};
static const struct wl_message zxdg_output_v1_events[] = {{"logical_position", "ii", zxdg_output_v1_event_0_types},
{"logical_size", "ii", zxdg_output_v1_event_1_types},
{"done", "", NULL},
{"name", "2s", zxdg_output_v1_event_3_types},
{"description", "2s", zxdg_output_v1_event_4_types}};
static const struct wl_interface zxdg_output_v1_interface = {"zxdg_output_v1", 3, 1, zxdg_output_v1_requests, 5, zxdg_output_v1_events};
static const struct wl_interface *wp_viewporter_request_1_types[] = {&wp_viewport_interface, &wl_surface_interface};
static const struct wl_message wp_viewporter_requests[] = {{"destroy", "", NULL},
{"get_viewport", "no", wp_viewporter_request_1_types}};
static const struct wl_interface wp_viewporter_interface = {"wp_viewporter", 1, 2, wp_viewporter_requests, 0, NULL};
static const struct wl_interface *wp_viewport_request_1_types[] = {NULL, NULL, NULL, NULL};
static const struct wl_interface *wp_viewport_request_2_types[] = {NULL, NULL};
static const struct wl_message wp_viewport_requests[] = {{"destroy", "", NULL},
{"set_source", "ffff", wp_viewport_request_1_types},
{"set_destination", "ii", wp_viewport_request_2_types}};
static const struct wl_interface wp_viewport_interface = {"wp_viewport", 1, 3, wp_viewport_requests, 0, NULL};
#endif
