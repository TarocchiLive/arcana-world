# Arcana World Overlay

[中文](README.md) · **English**

The desktop overlay shows room status and live chat in a fixed area of the screen. It is translucent, never takes focus, and lets mouse input pass through. It can also run on its own to show notices, a clock, or text from another program.

[Back to Arcana World](../../README.en.md)

## Two overlays

| Overlay | Highlights |
| --- | --- |
| Native overlay | Simple text display with adjustable font, colors, outline, and opacity |
| Elegant overlay | Adds message entrance animations, gradients, rounded corners, and shadows, with finer layout and motion settings; can hide when no new messages arrive |

Both overlays show the same content and keep separate appearance settings.

## Supported platforms

| Platform | Native overlay | Elegant overlay |
| --- | --- | --- |
| macOS (Apple silicon) | Supported | Supported |
| Windows x64 (10 1803 or later) | Supported | Supported |
| Windows ARM64 (10 1803 or later) | Supported | Not supported |
| Linux Wayland (KDE Plasma, niri, and other layer-shell desktops) | Supported | Supported |
| GNOME Wayland | Not supported | Requires an extension; see [GNOME Elegant overlay](../../docs/gnome-overlay.md) (Chinese) |
| Linux X11 | Not supported | Not supported |

On macOS the overlay appears on every desktop space and needs no Accessibility or Screen Recording permission. No platform guarantees visibility over lock screens, secure system surfaces, or exclusive fullscreen games.

## Turn on the overlay

1. Extract the entire release package and run `arcana-world` at its root (`arcana-world.exe` on Windows).
2. Open the Danmaku overlay page, choose Native overlay or Elegant overlay under Overlay style, and turn on the Overlay switch.
3. Check displays in Select overlay displays, and the events to show in Select overlay events.

WebUI offers the same settings under Outputs. The overlay appears on the computer running Arcana World and closes when the main program exits. Keep the main program, `LICENSE`, and the `libexec` directory together, and move them as a whole.

`arcana-world -overlay` or `-overlay=false` applies to the current launch only and does not change the saved switch.

### Content

Content mode in Overlay content and appearance offers Live danmaku, Room status, and Status and danmaku.

Chat stays in sync with the main chat list and takes up to its latest 100 selected events, also bounded by the main list's chat limit. The panel fills its height from the newest message upward; Elegant overlay also applies its Visible messages setting. Overlay event selections are independent of TTS. Turning off chat listening clears the overlay. To change how text is formatted, see the in-app help topic Customize overlay and TTS formats.

### Appearance

Overlay content and appearance adjusts position, size, fonts, colors, and opacity. Elegant overlay adds three groups: Elegant overlay text, Elegant overlay panel, and Elegant overlay motion.

- While the overlay runs, valid input appears on it immediately. Enter saves; Esc restores the saved value.
- Use ↑/↓ in numeric fields to step position, size, padding, and font size by 1, font weight by 100, and opacity by 0.01.
- Without live chat, turn on Sample preview on the Danmaku overlay page to see how each event type looks, without signing in. Samples are not saved to history or read aloud; turning preview off or leaving the page restores actual content.
- With Hide when no new messages on, Elegant overlay fades out after the content stays unchanged for the set number of seconds and fades back in on new messages.
- Elegant overlay can turn on Show in Dock on macOS or Show in taskbar on Windows to help OBS find the window. Capture still depends on system permissions and the capture method.

Restore appearance defaults resets only overlay appearance, keeping the content mode and switch.

### Displays

Select overlay displays accepts several displays and applies each change immediately; press `r` to refresh the list. Unchecking all hides the overlay. When a selected display disconnects, its overlay does not move to another screen and returns when the display reconnects.

### Overlay not visible

- The Danmaku overlay page shows the overlay state and latest error; the Logs page has details. After a failure, turn the Overlay switch off and on to retry.
- Check the selected displays, that chat listening is on, that the events are selected, and that block words are not hiding the messages.
- With automatic hiding on, the panel hides while there are no new messages.

## Standalone use

Overlay helpers are in the release package's `libexec` directory, or `bin/libexec` for local builds. Windows helpers add `.exe`.

| Overlay | Helper |
| --- | --- |
| Native overlay | `libexec/arcana-world-overlay-native` |
| Elegant overlay (Linux, Windows) | `libexec/arcana-world-overlay-qt` |
| Elegant overlay (macOS) | `libexec/Arcana World Elegant Overlay.app/Contents/MacOS/arcana-world-overlay-qt` |

Press `Ctrl+C` in the terminal to close, or add `-duration 30s` to close after 30 seconds. `-help` lists every option and its range. Options in this section do not apply when the main program manages the overlay.

### Position and appearance

```sh
./libexec/arcana-world-overlay-native -text 'Welcome to the stream' -anchor bottom-left -x 24 -y 24 -width 480 -height 200 -padding 20 -font-size 28 -background-alpha 0.25
```

| Option | Purpose |
| --- | --- |
| `-text` | Text to display |
| `-anchor` | Screen anchor; see below |
| `-x`, `-y` | Offsets; positive values move inward from an edge, or right/down when centered |
| `-width`, `-height` | Width and height |
| `-padding` | Padding; one value for all sides, or four values for top, right, bottom, left |
| `-font` | Font family; system default if omitted |
| `-font-size`, `-font-weight` | Font size; weight 100–900 |
| `-italic` | Italic text |
| `-outline=false` | Disable the text outline |
| `-text-alpha`, `-background-alpha` | Text and background opacity, 0–1; a background of 0 shows text only |

| Left | Center | Right |
| --- | --- | --- |
| `top-left` | `top` | `top-right` |
| `left` | `center` | `right` |
| `bottom-left` | `bottom` | `bottom-right` |

By default the overlay sits on the left edge, vertically centered, at 420 × 180 with font size 16. Position, size, padding, and font size use logical units that follow display scaling.

Text is top-aligned and wraps. On overflow, complete messages at the end take priority; messages are separated by newlines. Text updates never move or resize the overlay.

### Live text

```sh
# Clock updated every second
./libexec/arcana-world-overlay-native -text 'Current time' -clock
# Each line read replaces all text; the last line stays after input ends
printf '%s\n' 'New notice' | ./libexec/arcana-world-overlay-native -stdin
# Loop sample chat messages
./libexec/arcana-world-overlay-native -demo
```

Use only one of `-clock`, `-stdin`, and `-demo`. Each `-stdin` line can be up to 1 MiB.

### Displays

- `-list-displays`: print display IDs and names as JSON.
- `-displays '["ID1","ID2"]'`: show the same content on several displays.
- `-display` or `-output`: select one display. On macOS use `-display <ID>`; on Windows use `-display 2` (the second display) or `-output <device name>`; on Linux use `-output <output name>`, such as `DP-1`.

`-displays` cannot be combined with `-display` or `-output`.

### Elegant overlay options

These options work only with Elegant overlay:

| Option | Purpose |
| --- | --- |
| `-unlocked` | Allow dragging the overlay |
| `-reduced-motion` | Disable motion |
| `-idle-hide`, `-idle-seconds` | Hide when no new messages arrive, and the delay in seconds |
| `-show-in-taskbar` | Show in the macOS Dock or Windows taskbar |
| `-align`, `-history-scale`, `-latest-scale`, `-line-spacing`, `-message-spacing`, `-history-alpha`, `-max-messages`, `-max-lines` | Layout |
| `-gradient`, `-accent-start`, `-accent-end`, `-radius`, `-border-alpha`, `-shadow` | Gradient and panel |
| `-animation`, `-animation-ms`, `-animation-distance` | New-message animation |

## Build from source

From the project root:

```sh
make overlay      # Native overlay
make overlay-qt   # Elegant overlay
make desktop      # Main program, both overlays, and speech helper
```

`make desktop-native` builds the full package without Elegant overlay. Dependencies and Qt setup are covered in [Building from source](../../docs/building.md#浮层) (Chinese).
