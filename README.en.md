# Arcana World

<p align="center">
  <img src="assets/arcana-world.webp" alt="Arcana World — The World tarot card" width="180">
</p>

[![CI](https://github.com/TarocchiLive/arcana-world/actions/workflows/ci.yml/badge.svg)](https://github.com/TarocchiLive/arcana-world/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-GPL--3.0--only-blue)](LICENSE)
[![Release](https://img.shields.io/github/v/release/TarocchiLive/arcana-world?sort=semver)](https://github.com/TarocchiLive/arcana-world/releases/latest)

[中文](README.md) · **English**

Arcana World is a Go-based TUI alternative to bilibili LiveHime. It offers a way to go live on platforms where LiveHime cannot run directly, such as Linux, or when you encounter difficulties using the official client. Sign in and configure your broadcast from the terminal, then stream directly with OBS.

## Feature details

- QR login and saved accounts you can switch between.
- Start and stop broadcasts; edit the title, category, announcement, and cover.
- Crop and preview covers before uploading.
- Send stream settings to OBS and optionally start and stop streaming together.
- RTMP / SRT, Chinese and English interfaces, and proxy support.
- Cross-platform: available on macOS, Windows, and Linux.

## Install

Download the [latest release](https://github.com/TarocchiLive/arcana-world/releases/latest), extract it, and run:

| Platform | x64 | ARM64 |
| --- | --- | --- |
| macOS | [Intel](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-darwin-amd64.tar.gz) | [Apple Silicon](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-darwin-arm64.tar.gz) |
| Windows | [Download](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-windows-amd64.zip) | [Download](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-windows-arm64.zip) |
| Linux | [Download](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-linux-amd64.tar.gz) | [Download](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-linux-arm64.tar.gz) |

Or build from source with Go 1.26+:

```sh
git clone https://github.com/TarocchiLive/arcana-world.git
cd arcana-world
make build
./bin/arcana-world
```

Account credentials are saved in your system keyring. Linux users need an available, unlocked Secret Service.

## Go live

1. Open **Accounts** and scan the QR code with the Bilibili app.
2. Set your title, category, and cover on the **Room** page.
3. In OBS Studio's top menu, choose **Tools → WebSocket Server Settings → Enable WebSocket server** (check it) **→ Show Connect Info →** copy the server password **→ OK** ([official setup guide](https://obsproject.com/kb/remote-control-guide)).
   OBS 28 and later include this feature—no plugin installation is needed. For older versions, install an [obs-websocket 5.x plugin](https://github.com/obsproject/obs-websocket/releases) compatible with your OBS version.
4. In Arcana World's **Settings**, enter the server address (for example `ws://127.0.0.1:4455`) and the password you just copied.
5. Connect on the **OBS** page and enable **Auto-stream**.
6. Return to **Live** and start your broadcast.

Choose **Stop Bilibili live** when you are finished. With Auto-stream disabled, start and stop streaming in OBS manually.

If Bilibili asks for identity verification, follow the prompt and retry afterward.

### Keyboard shortcuts

| Key | Action |
| --- | --- |
| `↑↓` / `j k` | Select |
| `Enter` | Confirm |
| `Tab` / `1–7` | Switch pages |
| `PgUp` / `PgDn` | Scroll |
| `r` | Refresh |
| `Esc` | Back or cancel |
| `q` | Quit |

### Options

The interface follows your system language by default. To choose a language or data directory:

```sh
arcana-world --lang en                  # Use English
arcana-world --lang zh-CN               # Use Chinese
arcana-world --config-dir /path/to/data # Set the data directory
arcana-world --help                     # Show help
```

Default data directory: `~/.arcana/world`.

## Development

```sh
make run    # Run locally
make check  # Run tests and static checks
make build  # Build
```

Translations live in [`internal/i18n/locales`](internal/i18n/locales), grouped by language and module. Report problems through [Issues](https://github.com/TarocchiLive/arcana-world/issues), or contribute a Pull Request.

## Statement

This project is an independently developed open-source tool. Users are responsible for complying with applicable laws, regulations, and platform rules. This project is intended solely for learning and exchanging knowledge about Go and related frameworks. Please do not use it for other purposes.

## License

Licensed under [GPL-3.0-only](LICENSE).

## Acknowledgments

- [Radekyspec/StartLive](https://github.com/Radekyspec/StartLive): the source of this project's protocol implementations.
- [Bubble Tea](https://github.com/charmbracelet/bubbletea): the terminal UI framework.
