# Changelog

[中文](CHANGELOG.md) · **English**

Notable changes are listed by version, newest first. v0.1.5 is reserved for the next release and has not been published. Add its release date when publishing, then start the next version's entry.

## [v0.1.5] — Unreleased

### Added

- **Chat and history:** automatically listen to your room after sign-in, including chat, gifts, Super Chats (SC), and guard events. Browse local history, pause following, page through records, and switch rooms.
- **Event decoding:** support 177 registered commands, including newer protobuf gift, interaction, and high-energy ranking messages.
- **Display allowlist:** separate always-visible, `f`-only, and always-hidden messages. See the [chat display allowlist](docs/danmaku-whitelist.md) for individual rules.
- **New-message banner:** show a notification at the top while paused or browsing history. Press `Space` or `End` to return to the latest messages.
- **Nix support:** add Flake build, application, and check outputs with pinned build dependencies.
- **Nix development shells:** use `nix develop` (or `nix develop .#arcana-world`) for the independent TUI environment, and `nix develop .#arcana-overlay` for overlay dependencies with the Wayland build tag enabled on Linux. Development uses Go modules without generating `vendor/`.

### Changed

- Display messages oldest to newest and scroll to the bottom while following. Keep the Other-events toggle fixed at the top.
- Show joins and follows by default, shares and special follows with `f`, and hide mutual-follow notices. All 30 currently supported PK commands are `f`-only.
- Hide cross-room broadcasts, recommendations, offline-room lists, non-PK rankings, and similar notifications even with `f` enabled. Original business payloads are still saved.
- Retain history, raw business payloads, and operation logs for seven days with automatic cleanup. Messages with reliable identifiers support deduplication across restarts.
- Consolidate OBS connection settings on the dedicated OBS page; keep proxy and streaming protocol options on Settings.
- Add interface screenshots, simplify both READMEs, and move the detailed allowlist into `docs/`.
- Separate chat state management from rendering, unify room-switch resets, and remove redundant code and duplicate tests.

### Fixed

- Fix scrolling implicitly pausing live following and preventing new messages from appearing promptly.
- Preserve reading positions during pauses, paging, and asynchronous reads. Hidden notifications no longer trigger the new-message banner, and unread state does not leak between rooms.
- Fix pausing immediately after returning from a historical room to the live room potentially using the previous room's sequence number.
- Reject protobuf field numbers outside the valid range while continuing to accept valid unknown fields.

### Notes

- Messages missed while disconnected are not replayed after reconnection. Local history may be incomplete and is not a complete financial ledger.
- Decoding support does not imply visibility. Unlisted messages and payloads that fail to parse remain hidden.

## [v0.1.4] — 2026-09-14

### Added

- Add Chinese and English interfaces and messages, automatic system-language detection, and manual selection with `--lang`.
- Add x64 / ARM64 release packages for macOS, Windows, and Linux, covering six platform combinations.
- Add Chinese and English READMEs, the application icon, installation links, OBS setup instructions, and keyboard documentation.

### Changed

- Separate releases from regular CI, validate version tags, and build release packages after checks pass.
- Include the license and SHA-256 checksums with releases, with workflows for stable and prerelease versions.

## [v0.1.3] — 2026-09-13

### Added

- Introduce the terminal broadcast console with QR sign-in, saved-account switching, and broadcast start / stop controls.
- Support editing room titles, categories, announcements, and covers, including cover cropping and terminal previews.
- Support OBS WebSocket v5 connections, writing streaming settings, and automatically synchronizing stream start / stop.
- Support RTMP / SRT and network proxies. Store account credentials and OBS passwords in the system keyring.
- Add persistent configuration, operation logs, and command-line options including `--config-dir` and `--version`.
- Establish basic tests and CI, with a Linux x64 release archive and checksums.

[v0.1.5]: https://github.com/TarocchiLive/arcana-world/tree/dev
[v0.1.4]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.1.4
[v0.1.3]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.1.3
