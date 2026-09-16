# Arcana Overlay

[中文](README.md) · **English**

Arcana World's cross-platform desktop text overlay displays text in a fixed screen area without taking focus or capturing mouse input. Use it independently for notices, a clock, or continuously updated text, or launch it with Arcana World to display room status and live chat.

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

By default, the overlay is vertically centered on the left edge with zero horizontal and vertical offsets, a size of 420 × 180, and font size 16. Position and font size use logical units that follow system display scaling (DPI).

```sh
./bin/libexec/arcana-overlay -text 'Welcome to the stream' -anchor bottom-left -x 24 -y 24 -width 480 -height 200 -padding 20 -font-size 28 -background-alpha 0.25
```

| Option | Purpose |
| --- | --- |
| `-text 'Text'` | Content to display |
| `-anchor left` | Screen position; see the grid below |
| `-x 0 -y 0` | Horizontal and vertical offsets; positive values move inward from anchored edges, or right/down on centered axes |
| `-width 420 -height 180` | Overlay dimensions, both greater than 0 |
| `-padding 12,16,12,16` | Top, right, bottom, and left padding; one value applies to all sides |
| `-font 'Font name'` | Font family; omit to use the system default |
| `-font-size 16 -font-weight 500` | Font size and weight; weight ranges from 100 to 900 |
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
./bin/libexec/arcana-overlay -text 'Current time' -clock
```

Receive text piped from another program:

```sh
printf '%s\n' 'New notice' | ./bin/libexec/arcana-overlay -stdin
```

With `-stdin`, each input line replaces all displayed text. When input ends, the last line remains visible and the overlay stays open. Each line can contain approximately 1 MiB. This option cannot be combined with `-clock`.

### Monitor selection

By default, the overlay uses the monitor selected at startup. To choose one explicitly:

- macOS: `-display <display ID>`.
- Windows: `-display 2` for the second monitor, or `-output <device name>`.
- Linux: `-output DP-1`, using an actual output name from your desktop.

Do not use `-display` and `-output` together.

### Launch with Arcana World

Extract the entire portable bundle, run `arcana-world` at its root (`arcana-world.exe` on Windows), and enable Native overlay in Settings. You do not need to start the helper manually. Keep the main executable, `LICENSE`, and `libexec/arcana-overlay` (`.exe` on Windows) in their packaged locations, and move the whole folder together. After building locally with `make desktop`, you can also enable it from the command line:

```sh
./bin/arcana-world -overlay
```

By default, the overlay displays live chat and closes when the main program exits. Its content is empty before sign-in or when no messages are available. On Settings, toggle Native overlay or open Overlay content and appearance to change the content mode, position, size, padding, font, opacity, and monitor. Settings persist across launches. Startup failures do not block other features; Settings and Logs show the reason. Turn the overlay off and on again to retry.

Content modes are listed as Live chat, Room status, and Status and chat. Chat modes show up to six ordinary messages from the signed-in account's live room, independently of TUI history browsing, selected history room, or active page. Disabling chat listening clears overlay chat. Each refresh scans at most 256 recent new records, so heavy notification traffic can skip older chat messages; the saved records remain available on the Chat page.

`-overlay` and `-overlay=false` override the switch for this launch without changing saved settings. Using the switch in the TUI saves it. Use `-overlay-executable /path/to/arcana-overlay` for an executable in another location. The standalone appearance options above do not apply when the main program manages the overlay.

Saved values are not changed when defaults change. Restore default settings resets the entire Settings page, including the overlay, while keeping OBS configuration, its password, and Bilibili login information. Restore default appearance asks for confirmation, resets only overlay appearance while keeping its content mode and enabled state, and shows a completion message.

## Build and launch

The `arcana-world-<goos>-<goarch>.tar.gz` / `.zip` portable bundles contain the main executable and the overlay under `libexec`. Run source build commands from the **project root** with the project's required Go version installed.

macOS also requires Xcode Command Line Tools. Linux requires the Wayland and Pango/Cairo development libraries and `pkg-config`. On Debian/Ubuntu:

```sh
sudo apt-get install libwayland-dev libpango1.0-dev libcairo2-dev pkg-config
```

macOS / Linux:

```sh
make overlay
./bin/libexec/arcana-overlay -text 'Welcome to the stream'
```

Use `make desktop` to build `bin/arcana-world` and `bin/libexec/arcana-overlay` (`.exe` on Windows). `make build` still builds only the TUI and needs no graphics development libraries. The default Nix package includes the overlay: run `nix run -- -overlay`. Use `nix run .#arcana-world` for the TUI alone.

Windows (PowerShell):

```powershell
go build -o bin/libexec/arcana-overlay.exe ./cmd/arcana-overlay
.\bin\libexec\arcana-overlay.exe -text 'Welcome to the stream'
```

Press `Ctrl+C` in the launching terminal to close the overlay, or add `-duration 30s` to close it automatically after 30 seconds. The standalone commands above use local build paths; in a portable bundle, use `./libexec/arcana-overlay`, or `.\libexec\arcana-overlay.exe` on Windows.

## More options

```sh
./bin/libexec/arcana-overlay -help
```

The `-socket` option shown in help is used when the main program manages the overlay. You do not need to set it for normal standalone use.
