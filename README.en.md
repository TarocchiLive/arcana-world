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

## Help

Press `0` or open Help in the app for instructions and common questions. Run `arcana-world --help` for command-line options.

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
