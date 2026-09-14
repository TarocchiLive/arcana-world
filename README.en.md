# Arcana World

<p align="center">
  <img src="assets/arcana-world.webp" alt="Arcana World — The World tarot card" width="180">
</p>

[![CI](https://github.com/TarocchiLive/arcana-world/actions/workflows/ci.yml/badge.svg)](https://github.com/TarocchiLive/arcana-world/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-GPL--3.0--only-blue)](LICENSE)
[![Release](https://img.shields.io/github/v/release/TarocchiLive/arcana-world?sort=semver&cacheSeconds=300)](https://github.com/TarocchiLive/arcana-world/releases/latest)

[中文](README.md) · **English**

Arcana World is a Go-based TUI alternative to bilibili LiveHime. It offers a way to go live on platforms where LiveHime cannot run directly, such as Linux, or when you encounter difficulties using the official client. Sign in and configure your broadcast from the terminal, then stream directly with OBS.

## Preview

![Main menu](assets/arcana-world-1-en.png)
![Cover preview](assets/arcana-world-2-en.png)

## Feature details

- QR login and saved accounts you can switch between.
- Start and stop broadcasts; edit the title, category, announcement, and cover.
- Automatically crop covers and preview them in the terminal before uploading (requires a compatible terminal emulator; kitty or ghostty is recommended).
- Send stream settings to OBS and optionally start and stop streaming together.
- Automatically listen to your room after sign-in: chat, gifts, Super Chats, and guard purchases, with a toggle and local history.
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
| `Tab` / `1–8` | Switch pages |
| `PgUp` / `PgDn` | Scroll |
| `r` | Refresh |
| `Esc` | Back or cancel |
| `q` | Quit |

### Chat and history

Signing in or restoring a saved account automatically connects to that account's room. No manual Cookie, streamer identity code, or OBS connection is required. Press `8` for Chat; listening continues on other tabs.

| Key | Action |
| --- | --- |
| `s` / `Enter` | Toggle listening; disabling requires confirmation, persists across restarts, and keeps history |
| `Space` | Pause display / follow latest; pausing display does not stop an enabled listener |
| `[` / `]` | Older / newer history page, up to 100 records per page |
| `End` | Return to the active listener's latest records |
| `o` | Cycle rooms with local history, including while signed out |
| `f` | Show / hide other events; they are always stored and included in page counts |
| `r` | Refresh history and retry failures without interrupting a healthy connection |
| `PgUp` / `PgDn` | Scroll and pause following so new messages do not move the page |

History lives in `danmaku/history.db` under the data directory, separated by room. Messages commit in synchronous database transactions before display. Reliable event IDs deduplicate replays across restarts; identical text without a reliable ID is retained to avoid losing legitimate repeated messages. Both legacy and protobuf gifts are supported. Unknown or malformed business payloads are retained raw. Oversized text is shortened only for display; the full content stays in local history.

**Reliability limits:** the webpage API does not guarantee delivery or provide a reliable offline replay cursor. Network loss, app shutdown, disabling listening, platform restrictions, or messages the server never sends can leave gaps. This is not a zero-loss archive or a complete financial ledger. Session boundaries are recorded. Storage failures show a warning and retain pending messages for retry before switching accounts. Shutdown reports writes that still fail; forced termination, power loss, or damaged storage can lose uncommitted events.

**Local privacy:** chat, gifts, and raw business events are not automatically pruned; monitor disk usage. Deleted Super Chats are hidden in the UI, but raw content remains on disk. Restrictive permissions (Unix directories `0700`, database `0600`) are not encryption; do not share the database. Cookies and connection authentication packets are never written to history. Only one history writer can use a data directory at a time. A lock or storage error appears on Chat without disabling other live controls.

### Options

The interface follows your system language by default. To choose a language or data directory:

```sh
arcana-world --lang en                  # Use English
arcana-world --lang zh-CN               # Use Chinese
arcana-world --config-dir /path/to/data # Set the data directory
arcana-world --help                     # Show help
```

Default data directory: `~/.arcana/world`.

## TODO
- [ ] Fetch direct Bilibili stream URLs, connect to OBS automatically, and start broadcasting automatically.
- [x] Receive Bilibili live chat messages and provide gift and chat history views in the TUI.
- [ ] Add a Bilibili live chat overlay.
- [ ] Add LiveHime support, TTS chat readouts, and gift announcements.

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
- [xfgryujk/blivedm](https://github.com/xfgryujk/blivedm): reference for webpage chat protocol, WBI signing, and event structures (MIT).
- [Bubble Tea](https://github.com/charmbracelet/bubbletea): the terminal UI framework.
