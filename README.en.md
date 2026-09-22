# Arcana World

<p align="center">
  <img src="assets/arcana-world.webp" alt="Arcana World — The World tarot card" width="180">
</p>

[![CI](https://github.com/TarocchiLive/arcana-world/actions/workflows/ci.yml/badge.svg)](https://github.com/TarocchiLive/arcana-world/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-GPL--3.0--only-blue)](LICENSE)
[![Release](https://img.shields.io/github/v/release/TarocchiLive/arcana-world?sort=semver&cacheSeconds=300)](https://github.com/TarocchiLive/arcana-world/releases/latest)

[中文](README.md) · **English**

Arcana World is a Go-based bilibili live streaming and chat client.

It supports mobile QR-code sign-in, a floating chat overlay, and speech announcements across platforms.

Native support for Windows, Linux, and macOS.

## Preview

<p align="center">
  <img src="assets/arcana-world-01.webp" alt="Main menu" width="48%">
  <img src="assets/arcana-world-02.webp" alt="Overlay settings" width="48%">
  <img src="assets/arcana-world-03.webp" alt="Cover preview" width="48%">
  <img src="assets/arcana-world-04.webp" alt="Chat overlay" width="48%">
</p>

## Highlights

- **Scan to sign in, connect OBS, and go live with one click.**
- **Browse and send live chat in real time, with a click-through floating overlay that stays out of the way of your mouse and keyboard.**
- **High-quality chat, gift, and Super Chat announcements powered by Edge TTS, with zero resource overhead.**
- **Typical memory usage of 40 MB: lightweight, smooth, and stable.**

## Install

### Quick start

Download the [latest release](https://github.com/TarocchiLive/arcana-world/releases/latest) for your platform, such as `arcana-world-windows-amd64.zip`, extract the entire archive, and run `arcana-world` at its root (`arcana-world.exe` on Windows).

### Build from source

Building from source requires Go 1.26.8+ and the platform's [overlay build dependencies](cmd/arcana-world-overlay/README.en.md#build-and-launch). On Linux, the bundled audio helper also requires ALSA development libraries and `pkg-config` (`libasound2-dev pkg-config` on Debian/Ubuntu):

```sh
git clone https://github.com/TarocchiLive/arcana-world.git
cd arcana-world
make desktop
./bin/arcana-world
```

After building, run `bin/arcana-world` (`bin/arcana-world.exe` on Windows). For terminal-only use without the overlay, build with `make build`; graphics development libraries are not required. See [Arcana World Overlay](cmd/arcana-world-overlay/README.en.md#supported-platforms) for overlay platform support.

## Go live

1. Open **Accounts** and scan the QR code with the Bilibili app.
2. Set your title, category, and cover on the **Room** page.
3. In OBS Studio's top menu, choose **Tools → WebSocket Server Settings → Enable WebSocket server** (check it) **→ Show Connect Info →** copy the server password **→ OK** ([official setup guide](https://obsproject.com/kb/remote-control-guide)).
OBS 28 and later include this feature, so no plugin is needed. For older versions, install an [obs-websocket 5.x plugin](https://github.com/obsproject/obs-websocket/releases) compatible with your OBS version.
4. On Arcana World's **OBS** (`7`) page, enter the server address (for example `ws://127.0.0.1:4455`) and the password you just copied.
5. Connect on the same page and enable **Auto-stream**. Adjust proxy and streaming protocol options on **Settings** (`8`).
6. Return to **Live** and start your broadcast.

By default, exiting stops OBS streaming and closes the current account's Bilibili room, including broadcasts started from another client. On **Settings** (`8`), **Stop OBS streaming on exit** and **Close live room on exit** can be disabled independently. With both off, exiting performs neither action. These settings do not affect **Stop Bilibili live** on the Live page.

Switching to a different account (including QR sign-in) or deleting the current account asks for confirmation before stopping the old account's broadcast and associated OBS stream. If stopping fails, the account is not switched or deleted. Exit settings do not control this cleanup.

If Bilibili requests identity verification, complete it and try again.

### Chat and history

Press `2` to browse chat and history; listening continues on other pages.

- At startup, Chat shows the latest 10 messages from the past 24 hours and keeps up to 30 by default. The scrollbar shows your reading position.
- Press `/` or click the bottom input to compose up to 40 characters. `Enter` sends; `Esc` / `Tab` returns to browsing. Failed sends keep the draft.
- Use `PgUp` / `PgDn` or the mouse wheel to scroll, `Home` / `End` to jump to the top / latest message, and `r` to refresh / retry.
- Press `h` to search history by start and end time. Use `↑` / `↓` to switch time fields, `Enter` to confirm, and `Esc` to return. Your main reading position and draft are preserved.
- Adjust listening, additional events, and the main message limit on **Settings** (`8`).
- Drag to select text in chat, history, logs, or notifications. Press `y` to copy or `Esc` to clear the selection; shortcuts appear in the footer.

See the [chat display allowlist](docs/danmaku-whitelist.md) for the default message types. Unlisted messages stay hidden even with additional events enabled, but their original data is still saved.

History is stored per room in `danmaku/history.db` under the data directory; history and operation logs are retained for seven days and cleaned automatically. Messages missed while disconnected are not fetched later to fill in the database.

### Desktop overlay

Provides a focus-free, click-through, semi-transparent desktop text overlay. Overlay and TTS have independent event selections; all message types shown by default are initially selected. Press `Enter` or `Space` to toggle an event. Output templates currently support Chinese only. The TTS tab offers voice selection, preview, on/off controls, and stopping playback with queue clearing. Only new events are spoken after enabling TTS; history is not replayed. Speech synthesis requires an internet connection and the bundled companion executable.

See [Arcana World Overlay](cmd/arcana-world-overlay/README.en.md) for details.

### Options

The interface uses your system language by default. You can choose a language or data directory with:

```sh
arcana-world --lang en                  # Use English
arcana-world --lang zh-CN               # Use Chinese
arcana-world --config-dir /path/to/data # Set the data directory
arcana-world --help                     # Show help
```

Default data directory: `~/.arcana/world`.

## TODO
- [x] Fetch direct Bilibili stream URLs, connect to OBS, and start broadcasts automatically.
- [x] Receive Bilibili live chat messages and browse gifts and chat history in the TUI.
- [x] Add a Bilibili live chat overlay.
- [x] Add TTS chat readouts and gift announcements.
- [x] Switch the overlay between monitors without restarting.
- [x] Add mouse support to the TUI.
- [x] Refactor and improve the TUI layout and appearance.
- [x] Send live chat messages from the terminal.
- [ ] Refactor and improve the overlay layout and appearance.
- [ ] Support the iTerm2 image protocol.
- [ ] Support resizing and cropping cover images in the terminal.
- [ ] Check for new versions.
- [ ] Support package repositories such as Scoop, Homebrew, and nixpkgs.
- [ ] Add a bot protocol interface.
- [?] GUI / WebUI support (under consideration).

## Development

```sh
make run    # Run locally
make check  # Run tests and static checks
make build  # Build
```

Translations live in [`internal/i18n/locales`](internal/i18n/locales), grouped by language and module. Report problems through [Issues](https://github.com/TarocchiLive/arcana-world/issues), or contribute a Pull Request.

## Statement

This project is an independently developed open-source tool. Follow applicable laws, regulations, and platform rules. Use it only to learn about and exchange knowledge on Go and related frameworks.

## License

Licensed under [GPL-3.0-only](LICENSE).

## Acknowledgments

- [Radekyspec/StartLive](https://github.com/Radekyspec/StartLive)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [Bubbles](https://github.com/charmbracelet/bubbles)
- [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- [edge-tts](https://github.com/rany2/edge-tts)
- [edge-tts-go](https://github.com/wujunwei928/edge-tts-go)
