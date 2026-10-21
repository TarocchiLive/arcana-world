# Arcana World Overlay

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
| X11 desktops | Elegant overlay supports X11 through xcb; Native overlay does not |

Visibility over lock screens, secure system surfaces, or exclusive fullscreen games is not guaranteed. Normal display on macOS does not require Accessibility or Screen Recording permission.

## Common operations

### Position and appearance

By default, the overlay is vertically centered on the left edge with zero horizontal and vertical offsets, a size of 420 × 180, and font size 16. Position and font size use logical units that follow system display scaling (DPI).

```sh
./bin/libexec/arcana-world-overlay-native -text 'Welcome to the stream' -anchor bottom-left -x 24 -y 24 -width 480 -height 200 -padding 20 -font-size 28 -background-alpha 0.25
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
| `-outline=false` | Disable the text outline; enabled by default |
| `-text-alpha 1` | Text opacity: 0 is fully transparent, 1 is opaque |
| `-background-alpha 0.35` | Background opacity; set to 0 to show text only |

Position names:

| Left | Center | Right |
| --- | --- | --- |
| `top-left` | `top` | `top-right` |
| `left` | `center` | `right` |
| `bottom-left` | `bottom` | `bottom-right` |

Position, size, and padding use logical units that follow display scaling. All text wraps to the available width with a fixed line height determined by the configured font. On overflow, the overlay prefers complete messages at the end, treating newline-separated paragraphs as messages. If no nonempty message starts within the visible range, it shows the last complete lines of the long message. Text is top-aligned, with unused height left blank below it. No text is drawn if the content area cannot fit one line. Increase the height or reduce the font size to show more lines. Text updates do not move or resize the overlay.

### Live updates

Show a clock that updates every second:

```sh
./bin/libexec/arcana-world-overlay-native -text 'Current time' -clock
```

Receive text piped from another program:

```sh
printf '%s\n' 'New notice' | ./bin/libexec/arcana-world-overlay-native -stdin
```

With `-stdin`, each input line replaces all displayed text. When input ends, the last line remains visible and the overlay stays open. Each line can contain approximately 1 MiB. This option cannot be combined with `-clock`.

### Monitor selection

By default, the overlay uses the monitor selected at startup. To choose one explicitly:

- macOS: `-display <display ID>`.
- Windows: `-display 2` for the second monitor, or `-output <device name>`.
- Linux: `-output DP-1`, using an actual output name from your desktop.

Do not use `-display` and `-output` together.

In the main program, open Overlay → Select overlay displays to toggle each monitor by name. Click a row or press `Enter` / `Space` to save and apply immediately without restarting the overlay; press `r` to refresh the list. Deselecting every display hides all overlays. A disconnected selected display stays hidden rather than moving to another screen, and reappears when reconnected.

For standalone use, `-list-displays` prints a JSON list of display names and stable IDs. Use `-displays '["ID1","ID2"]'` to show the same content on multiple monitors. Do not combine it with `-display` or `-output`.

### Launch with Arcana World

Extract the entire portable bundle and run `arcana-world` at its root (`arcana-world.exe` on Windows). On the Overlay tab, select Native overlay or Elegant overlay and enable it. Keep the main executable, `LICENSE`, and the entire `libexec` directory together. You do not need to start helpers manually. After building locally with `make desktop`, you can also enable the selected overlay from the command line:

```sh
./bin/arcana-world -overlay
```

By default, the overlay displays live events and closes when the main program exits. Its content is empty until the main chat list loads or when no displayable events are available. On the Overlay tab, enable the selected backend or open Overlay content and appearance to change the content mode, position, size, padding, font, and opacity. Use Select overlay displays to choose the monitors. Settings persist across launches. Startup failures do not block other features; Overlay and Logs show the reason. Turn the overlay off and on again to retry.

Overlay content and appearance shows settings for the selected style and retains each style's preferences. Native overlay keeps content mode and Restore default appearance on its first screen, with other options under Layout, Text, and Colors and opacity. Elegant overlay provides text, panel, and motion settings. Esc goes back one level; saving or canceling an edit returns to the same field. Each group remembers its selection until you close settings.

Without live messages, enable Sample preview on the Overlay tab to check wrapping, fonts, colors, size, and motion without signing in. First enable the selected overlay and wait until it is ready. The looping preview scrolls in a message every 1.5 seconds, showing regular and guard-tier chat, Super Chats, gifts, gift combos, Captain subscriptions, arrivals, and likes in the interface language, with every line marked as a sample. Samples are not added to history or read by TTS, and the preview switch is not saved. Turning preview off, leaving the Overlay tab, or disabling the overlay ends the preview. Actual content then includes any events received while previewing.

While the overlay is running, valid values in appearance input fields preview immediately, including position offsets, size, padding, fonts, colors, and opacity. Press Enter to save or Esc to restore saved values. Invalid input keeps the last valid preview. Previewing does not write configuration; combine it with Sample preview to adjust the appearance.

Use ↑/↓ in numeric fields to adjust position, size, padding, or font size by 1, font weight by 100, and opacity by 0.01 (rounded to two decimal places). Invalid values and steps beyond the allowed range leave the input unchanged. Left/right still move the cursor, and direct typing remains available. Press Enter to save or Esc to cancel stepped changes.

Content modes are listed as Live chat, Room status, and Status and chat. Chat modes share the TUI main chat list's data and refresh results without loading additional history. They display the latest six selected events from that list; the visible portion depends on wrapping and available height. At startup, both use the main list's limit of up to 10 messages from the past 24 hours. New messages, withdrawals, and removal of older entries update together. The interface language selects `templates/overlay.zh-CN.yaml` or `templates/overlay.en.yaml` in the configuration directory. Edit the corresponding external YAML file to customize templates, then save and restart to apply changes. Overlay event selections are independent of TTS. Browsing history or switching pages does not change the overlay's data source. Disabling chat listening clears overlay events.

`-overlay` and `-overlay=false` override the switch for this launch without changing saved settings. Using the switch in the TUI saves it. Use `-overlay-executable /path/to/arcana-world-overlay-native` for a helper in another location; it must match the selected backend. The standalone appearance options above do not apply when the main program manages the overlay.

Custom appearance settings are preserved. Elegant overlay's old default text style migrates to equal font sizes, no latest-message gradient, and 95% history text opacity, without changing event colors. Restore default settings resets application preferences, overlay settings, TTS voice, and event selections while keeping OBS configuration, its password, and Bilibili login information. Restore default appearance asks for confirmation, resets only overlay appearance while keeping its content mode and enabled state, and shows a completion message.

## Build and launch

The `arcana-world-<goos>-<goarch>.tar.xz` / `.zip` portable bundles contain the main executable and the overlay under `libexec`. Run source build commands from the **project root** with the project's required Go version installed.

Native overlay also requires Xcode Command Line Tools on macOS, or Wayland and Pango/Cairo development libraries and `pkg-config` on Linux. Full desktop builds additionally require the dependencies under [Elegant overlay build and runtime](#elegant-overlay-build-and-runtime) below. Install the native overlay dependencies on Debian/Ubuntu with:

```sh
sudo apt-get install libwayland-dev libpango1.0-dev libcairo2-dev pkg-config python3 patchelf xkb-data fontconfig-config
```

macOS / Linux:

```sh
make overlay
./bin/libexec/arcana-world-overlay-native -text 'Welcome to the stream'
```

`make desktop` builds the TUI, both overlay helpers, speech helper, and bundled graphics runtime. `make desktop-native` explicitly omits Elegant overlay. `make build` builds only the CGO-free TUI. Nix defaults to the full desktop package; `nix run .#arcana-world-desktop-native -- -overlay` selects the native-only package. The full macOS Nix package requires nixpkgs with Qt >= 6.11 and reports an error with older Qt.

See `bash scripts/build.sh --help` for build options. Both Linux backends deploy their runtime libraries; rebuilds remove stale dependencies, and switching to `desktop-native` retains shared files still needed by the native backend. Use a new output directory for older builds without an ownership manifest. Before redistributing local Nix builds, supply complete dependency licenses and `SOURCE.txt` through `LINUX_LICENSE_DIR`; official releases collect Ubuntu package copyright notices.

Windows (PowerShell):

```powershell
go build -o bin/libexec/arcana-world-overlay-native.exe ./cmd/arcana-world-overlay
.\bin\libexec\arcana-world-overlay-native.exe -text 'Welcome to the stream'
```

Press `Ctrl+C` in the launching terminal to close the overlay, or add `-duration 30s` to close it automatically after 30 seconds. In a portable bundle, standalone helper paths start with `./libexec/` rather than `./bin/libexec/`.

## More options

```sh
./bin/libexec/arcana-world-overlay-native -help
```

The `-socket` option shown in help is used when the main program manages the overlay. You do not need to set it for normal standalone use.

## Elegant overlay build and runtime

Elegant overlay uses Qt Widgets internally. Its helper is `libexec/arcana-world-overlay-qt` (`.exe` on Windows); the native helper is `libexec/arcana-world-overlay-native`. Both use protocol v4 and reject a host selecting the other backend. The only program users need to launch is `arcana-world`.

Full release bundles include both backends on macOS amd64/arm64, Linux amd64/arm64, and Windows amd64. Windows 386 and arm64 retain native-only bundles because the release toolchain has no matching Qt6 MinGW build. Selecting Elegant overlay in a native-only bundle reports the missing component; it never falls back to native.

Build on the target OS and architecture with CMake >= 3.24, Python >= 3.9, a C++17 compiler, and matching Qt development libraries:

- macOS: install Xcode Command Line Tools, `brew install cmake ninja`, and Python 3. Run `source scripts/setup-macos-qt.sh` to build and cache reduced shared Qt 6.11.0 and export the build environment. Set `QT_BUILD_PREFIX` to choose the SDK installation directory and `BUILD_JOBS` to control build concurrency. An existing Qt >= 6.11 SDK also works: point `CMAKE_PREFIX_PATH` to its prefix and `QTPATHS` to `bin/qtpaths`. Stable native display IDs require this minimum version.
- Linux: Qt >= 6.5 (Core/Gui/Widgets/WaylandClient), LayerShellQt >= 6.3, Wayland development libraries, and `patchelf`. The release setup script builds Qt 6.8.3 and LayerShellQt 6.3.6 on Ubuntu 24.04 and caches the installation. The release glibc baseline remains Ubuntu 24.04. `scripts/setup-linux-qt.sh` installs build prerequisites with sudo; outside CI, export its installation prefix as `CMAKE_PREFIX_PATH`, put its `bin` on PATH, and set `QTPATHS` to `bin/qtpaths`.
- Windows amd64: install MSYS2 UCRT64 GCC, CMake, Ninja, and Python, plus curl, tar, and native Windows amd64 Go. Run `source scripts/setup-windows-qt.sh` in a UCRT64 shell to build and cache a reduced shared Qt 6.8.3 and export the build environment; Go must be on PATH. Set `QT_BUILD_PREFIX` to choose the SDK installation directory and `BUILD_JOBS` to control build concurrency. Use the same UCRT64 compiler for Qt and cgo; do not mix MSVC Qt with MinGW.

SDK installation and work directories must be empty or created by the current script. A toolchain or configuration change clears and rebuilds both directories. Older directories without an ownership marker are preserved; choose new empty directories instead. Each platform script lists its options with `--help`.

`make overlay-qt` builds and deploys the Elegant helper. Deployment copies Qt plugins and the transitive shared-library dependencies into `libexec`, rewrites macOS/Linux loader paths, and ad-hoc signs modified macOS binaries. Windows ships only `qwindows.dll` and required Qt and MinGW DLLs. Third-party notices are combined in `libexec/licenses/THIRD_PARTY_NOTICES.txt`, with source links in the adjacent `SOURCE.txt`. Linux bundles LayerShellQt and Wayland client dependencies but uses the system's glibc, kernel, display server, and graphics drivers. Preserve the entire directory when moving or archiving the program.

On macOS the helper resides in a private `Arcana World Elegant Overlay.app/Contents/MacOS` bundle; the outer helper name is a relative symbolic link. The main program resolves the real path before launch and display enumeration so macOS reads the product name and initial background policy. Dock visibility can still change at runtime. For standalone debugging, execute `"./bin/libexec/Arcana World Elegant Overlay.app/Contents/MacOS/arcana-world-overlay-qt"` directly; launching through the symbolic link loses the application identity.

Both overlays in Linux release bundles use Wayland. Elegant overlay uses real LayerShellQt surfaces and requires a compositor exposing layer-shell; GNOME/Mutter is unsupported.

Linux portable bundles include the shared libraries, XKB data, and font configuration needed by both overlays, without manual library-path setup. Fonts come from system or user installations; explicit `XKB_CONFIG_ROOT`, `FONTCONFIG_FILE`, and `FONTCONFIG_PATH` settings are preserved.

For a standalone preview, run `./bin/libexec/arcana-world-overlay-qt -demo -duration 30s`. `-demo`, `-clock`, and `-stdin` are mutually exclusive. `-unlocked` permits dragging; `-reduced-motion` disables motion. Both options are standalone Elegant-overlay options, not managed `-socket` options. Elegant overlay creates no menu-bar/system-tray icon or context menu.

Elegant appearance flags are backend-neutral: `-align`, `-history-scale`, `-latest-scale`, `-line-spacing`, `-message-spacing`, `-history-alpha`, `-max-messages`, `-max-lines`, `-gradient`, `-accent-start`, `-accent-end`, `-radius`, `-border-alpha`, `-shadow`, `-animation`, `-animation-ms`, and `-animation-distance`. Run the Elegant helper with `-help` for ranges. Managed mode uses saved settings instead.

The overlay page's second item, directly after the enable toggle, selects Native overlay or Elegant overlay and explains their appearance and control differences. Elegant overlay's appearance settings include Show in Dock on macOS and Show in taskbar on Windows. The preference is saved as `overlay.qt.show_in_taskbar` (default `false`), sent to the helper as `config.qt.show_in_taskbar`, and applied live. Native overlay ignores this preference. On Linux, both X11 and Wayland leave it inactive and retain the saved value; Wayland continues to use layer-shell even when the saved value is `true`.

For standalone use, `--show-in-taskbar` enables this setting on the Elegant helper. The native helper rejects the flag, including `--show-in-taskbar=false`; managed `-socket` mode accepts no standalone flags. Dock/taskbar visibility may help OBS discover the application or window, but capture still depends on permissions and the selected capture mode. It does not create a tray icon.
