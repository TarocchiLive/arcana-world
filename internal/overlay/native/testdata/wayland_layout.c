#include "../wayland_linux.c"
#include <assert.h>

void arcanaWaylandReady(uintptr_t handle) { (void)handle; }

static int natural_height(const char *text, int width) {
    cairo_surface_t *image = cairo_image_surface_create(CAIRO_FORMAT_ARGB32, 1, 1);
    cairo_t *cr = cairo_create(image);
    PangoLayout *layout = pango_cairo_create_layout(cr);
    PangoFontDescription *font = pango_font_description_new();
    pango_font_description_set_family(font, "sans");
    pango_font_description_set_absolute_size(font, 18 * PANGO_SCALE);
    pango_layout_set_font_description(layout, font);
    pango_layout_set_width(layout, width * PANGO_SCALE);
    pango_layout_set_wrap(layout, PANGO_WRAP_WORD_CHAR);
    pango_layout_set_text(layout, text, -1);
    int height;
    pango_layout_get_pixel_size(layout, NULL, &height);
    pango_font_description_free(font);
    g_object_unref(layout);
    cairo_destroy(cr);
    cairo_surface_destroy(image);
    return height;
}

static struct pixels render(const char *text, int width, int height, int padding_right, int padding_bottom) {
    struct arcana_wayland_config config = {
        .text = (char *)text, .family = "sans", .font_size = 18,
        .weight = PANGO_WEIGHT_NORMAL, .text_alpha = 1,
        .padding_right = padding_right, .padding_bottom = padding_bottom,
    };
    struct arcana_wayland state = {.config = &config, .width = width, .height = height};
    struct pixels pixels = {.width = width, .height = height};
    pixels.stride = cairo_format_stride_for_width(CAIRO_FORMAT_ARGB32, width);
    pixels.size = (size_t)pixels.stride * height;
    pixels.data = malloc(pixels.size);
    assert(pixels.data);
    memset(pixels.data, 0xa5, pixels.size);
    assert(paint_pixels(&state, &pixels));
    assert(!state.error[0]);
    return pixels;
}

// Rendering into a short box must equal the tail of the same naturally wrapped
// text in a box large enough for every line. Adding a paragraph must not change
// how existing paragraphs wrap or hide the newest paragraph behind old ones.
static void check_tail(const char *text, int width, int height) {
    int full_height = natural_height(text, width);
    int offset = full_height > height ? full_height - height : 0;
    if (!offset) full_height = 2 * height;
    struct pixels full = render(text, width, full_height, 0, 0);
    struct pixels clipped = render(text, width, height, 0, 0);
    unsigned char *expected = (unsigned char *)full.data + offset * full.stride;
    // Cairo can interpolate clipped bitmap emoji a few alpha/color levels
    // differently; missing lines or ellipsized glyphs differ far beyond this.
    for (size_t i = 0; i < clipped.size; ++i) {
        int delta = abs((int)((unsigned char *)clipped.data)[i] - expected[i]);
        if (delta > 4) {
            fprintf(stderr, "wrapped text mismatch: width=%d height=%d natural=%d byte=%zu delta=%d text=%s\n",
                width, height, full_height, i, delta, text);
            abort();
        }
    }
    free(full.data);
    free(clipped.data);
}

static void check_empty_content(int padding_right, int padding_bottom) {
    struct pixels pixels = render("中文 emoji 🎉 newest", 120, 60, padding_right, padding_bottom);
    for (size_t i = 0; i < pixels.size; ++i) assert(((unsigned char *)pixels.data)[i] == 0);
    free(pixels.data);
}

int main(void) {
    const char *long_message = "A long message wraps across several visual lines before another message arrives.";
    int line_height = natural_height("latest", 120);
    assert(natural_height(long_message, 120) > 2 * line_height);
    check_tail(long_message, 120, 2 * line_height);
    check_tail("A long message wraps across several visual lines before another message arrives.\nlatest", 120, 2 * line_height);
    check_tail("第一条中文弹幕很长，需要按照可用宽度自动换行。🎉🌍\n最新消息 🚀", 120, 2 * line_height);
    check_tail("old\nnewest", 120, 1);
    check_tail("short", 120, 3 * line_height);
    check_tail("", 120, line_height);
    check_empty_content(120, 0);
    check_empty_content(0, 60);
    puts("native layout: wrapped tails, appended messages, Unicode and clipping passed");
    return 0;
}
