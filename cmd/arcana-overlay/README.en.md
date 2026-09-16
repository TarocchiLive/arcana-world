# Arcana Overlay

[中文](README.md) · **English**

Arcana World's cross-platform desktop text overlay displays text in a fixed screen area without taking focus or capturing mouse input. Use it independently for notices, a clock, or continuously updated text, or launch it with Arcana World to display the room title and live status. Bilibili live chat is not connected yet.

[Back to Arcana World](../../README.en.md)

## Features

- Live text updates with Chinese text and automatic wrapping.
- Place the overlay in nine positions with adjustable offsets, size, and padding on each side.
- Choose the font family, size, weight, and italic style.
- Set text and background opacity separately, or show text without a background.
- Select a monitor; switching foreground applications does not move the overlay.

## Supported platforms

| Platform | Support |
| --- | --- |
| macOS | Supported, including display across desktop Spaces |
| Windows | Windows 10 1803 and later |
| Linux | Wayland desktops such as KDE Plasma and niri |
| GNOME | GNOME Mutter lacks layer-shell and is not currently compatible |
| X11 desktops | No support planned at present |

Visibility over lock screens, secure system surfaces, or exclusive fullscreen games is not guaranteed. Normal display on macOS does not require Accessibility or Screen Recording permission.

## Common operations

### Position and appearance

By default, the overlay is vertically centered on the left side of the screen, 32 units from the left edge, and 420 × 180 units in size.

```sh
./bin/arcana-overlay -text 'Welcome to the stream' -anchor bottom-left -x 24 -y 24 -width 480 -height 200 -padding 20 -font-size 28 -background-alpha 0.25
```

| Option | Purpose |
| --- | --- |
| `-text 'Text'` | Content to display |
| `-anchor left` | Screen position; see the grid below |
| `-x 32 -y 0` | Horizontal and vertical offsets; positive values move inward from anchored edges, or right/down on centered axes |
| `-width 420 -height 180` | Overlay dimensions, both greater than 0 |
| `-padding 12,16,12,16` | Top, right, bottom, and left padding; one value applies to all sides |
| `-font 'Font name'` | Font family; omit to use the system default |
| `-font-size 22 -font-weight 500` | Font size and weight; weight ranges from 100 to 900 |
| `-italic` | Use italic text |
| `-text-alpha 0.9` | Text opacity: 0 is fully transparent, 1 is opaque |
| `-background-alpha 0.3` | Background opacity; set to 0 to show text only |

Position names:

| Left | Center | Right |
| --- | --- | --- |
| `top-left` | `top` | `top-right` |
| `left` | `center` | `right` |
| `bottom-left` | `bottom` | `bottom-right` |

Position, size, and padding use logical units that follow display scaling. Text wraps within the overlay, and content exceeding its height may be clipped. Increase the height or reduce the font size if needed.

### Live updates

Show a clock that updates every second:

```sh
./bin/arcana-overlay -text 'Current time' -clock
```

Receive text piped from another program:

```sh
printf '%s\n' 'New notice' | ./bin/arcana-overlay -stdin
```

With `-stdin`, each input line replaces all displayed text. When input ends, the last line remains visible and the overlay stays open. Each line can contain approximately 1 MiB. This option cannot be combined with `-clock`.

### Monitor selection

By default, the overlay uses the monitor selected at startup. To choose one explicitly:

- macOS: `-display <display ID>`.
- Windows: `-display 2` for the second monitor, or `-output <device name>`.
- Linux: `-output DP-1`, using an actual output name from your desktop.

Do not use `-display` and `-output` together.

### Launch with Arcana World

Place the overlay executable beside the main executable, then enable it:

```sh
./bin/arcana-world -overlay
```

The overlay displays the room title and live status and closes when the main program exits. Startup failures do not block other features; check the Logs page for details.

In this mode, the main program manages position and appearance; the options above apply only to standalone use.

## Build and launch

`arcana-overlay` is a standalone native program. The source tree includes it, but the main program's release packages do not; build it separately. Run the commands from the **project root** with the project's required Go version installed.

macOS also requires Xcode Command Line Tools. Linux requires the Wayland and Pango/Cairo development libraries and `pkg-config`. On Debian/Ubuntu:

```sh
sudo apt-get install libwayland-dev libpango1.0-dev libcairo2-dev pkg-config
```

macOS / Linux:

```sh
make overlay
./bin/arcana-overlay -text 'Welcome to the stream'
```

Windows (PowerShell):

```powershell
go build -o bin/arcana-overlay.exe ./cmd/arcana-overlay
.\bin\arcana-overlay.exe -text 'Welcome to the stream'
```

Press `Ctrl+C` in the launching terminal to close the overlay, or add `-duration 30s` to close it automatically after 30 seconds. The commands above use macOS / Linux paths; on Windows, use `.\bin\arcana-overlay.exe` instead.

## More options

```sh
./bin/arcana-overlay -help
```

The `-socket` option shown in help is used when the main program manages the overlay. You do not need to set it for normal standalone use.
