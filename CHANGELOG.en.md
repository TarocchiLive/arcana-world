# Changelog

[中文](CHANGELOG.md) · **English**

Versions are listed newest first.

## Unreleased

### Added

- Add “Ignore myself in viewer list” to Settings, enabled by default. Viewer counts exclude the current account, and viewer details hide it. Turning this off restores the original counts and list; Fleet is unchanged.

## [v0.3.0-alpha.3]: 2026-09-24 (prerelease)

### Changed

- Change the default mute duration in speaker profiles from one hour to two hours, with matching confirmation and help text. Permanent room mutes are unchanged.

## [v0.3.0-alpha.2]: 2026-09-24 (prerelease)

### Added

- Add Viewers (`u`) and Fleet (`g`) pages to Chat, with clickable entries, refresh, scrolling, and up to 20 users each. Viewers shows contribution scores; Fleet lists paid members and Governor, Admiral, or Captain tiers whether present or not. Show fan medals and hide UIDs for anonymous members.
- Add moderator (`m`) and live-room blocklist (`b`) menus for listing users, appointing or dismissing moderators, and applying or lifting permanent room mutes. Look up a username or UID, check the avatar and identity, then confirm the action. Record results without changing the account blocklist.
- Click a chat speaker's name to view their avatar, fan medal, and UID in a popup. Mute them for one hour, add them to the room blocklist, appoint them as moderator, or open their profile. The first three actions require confirmation; dragging a name still selects text.
- Show the viewer count beside live status, refreshed every 30 seconds, with an unknown state when unavailable.

### Changed

- Use native Kitty/Ghostty graphics for square avatars in profiles and confirmation dialogs, sized to terminal cell pixels, with character previews on other terminals. Failed avatar loads do not prevent confirmation by name and UID; clear images when closing popups or exiting.
- Consolidate usage instructions in the in-app Help and `--help`, with additional guidance on OBS connections, chat controls, and data directories.

### Fixed

- Fix missing left borders, shifted input cursors, and misaligned notifications on 32-bit platforms, including Windows 386.
- Fix historical name clicks targeting the wrong user after live messages arrive. Preserve drafts, historical content, and reading position when closing member lists or speaker popups.
- Fix stale viewer counts after returning to a room and moderation state after repeated confirmation or cancellation. Viewer-count requests no longer block other broadcast operations.
- Fix expired history cleanup failures preventing chat from loading.
- Identify the data directory and explain how to proceed when another instance is using it.

### Upgrade notes

- This is the second 0.3.0 alpha prerelease. Replace both the main executable and the complete `libexec` directory.
- Back up the data directory before upgrading. The upgraded history database cannot be used directly with 0.2.x.

## [v0.2.2]: 2026-09-24

### Fixed

- Fix automatic cleanup failing when expired history records have lost their original message references, preventing chat history from loading and the listener from starting. Cleanup removes only records older than seven days and preserves events and original messages within the retention period.

### Upgrade notes

- This is a patch for 0.2.1 without the new features from 0.3.0 alpha. Replace the complete release archive; there is no need to delete the history database.
- Do not use 0.2.2 with a data directory already upgraded by 0.3.0 alpha. Continue using the alpha development version with the same fix.

## [v0.3.0-alpha.1]: 2026-09-22 (prerelease)

### Added

- Add a wrapping input at the bottom of the chat frame. Press `/` or click to compose up to 40 characters, `Enter` to send, and `Esc` / `Tab` to return to browsing. Failed sends keep the draft.
- Set and save TTS volume from 0 to 100 on the TTS tab, with a default of 80 and 0 for mute. Previews and event announcements use the same volume.
- Choose and save one of five TUI themes in Settings: Lumen, Midnight, Nord, Ember, or Paper. The selection persists across restarts.
- Use the left mouse button to navigate pages, activate menus and pickers, position input cursors, submit forms, and confirm actions. Scroll long lists and content with the mouse wheel.
- Show a compact Arcana / WORLD wordmark and a slow flowing highlight in large windows. Remove the Earth animation and keep header and footer heights consistent across pages and submenus.
- Add Appearance settings for slow motion, a compact header, and warnings-only notifications, saved across restarts.
- Confirm before quitting with `q` or `Ctrl+C`, with Cancel selected by default. Canceling preserves unsaved input and the previous menu.
- In terminals supporting pointer-shape control, use an arrow over empty areas, a hand over actions, and a text pointer over inputs.
- Highlight actionable items on hover without activating them or changing keyboard selection.
- Split Help into six topics: first stream, OBS connection, picture and streaming troubleshooting, chat history, overlay and speech, and controls and settings. Read each topic separately and return to the directory.
- Drag to select body text in chat, history, logs, and notifications. Press `y` to copy or `Esc` to clear, with a footer-only hint and no copy button or automatic copy. `Ctrl+C` still opens quit confirmation.

### Changed

- Upgrade to Lip Gloss v2, Bubble Tea v2, and Bubbles v2, and rebuild all TUI pages, forms, and pickers. Use thin rounded borders, layered backgrounds, and consistent focus states, with grouped sidebar navigation on wide terminals and compact navigation on narrow ones.
- Rebuild Chat around the latest 10 messages from the past 24 hours at launch, with a default maximum of 30 messages and a scrollbar showing the reading position. Move listening, additional events, and the message limit to Settings. Replace room switching and history paging with a separate time-range browser that preserves the main frame and draft.
- Use Up / Down to switch between history start and end time fields, alongside Tab and Enter. Group all Settings toggles before the other options.
- Correct input cursor placement for CJK text, scrolling, and window resizing to improve IME candidate positioning across chat, forms, title editing, category search, and history time fields.
- Shorten the first-stream guide to four steps: sign in and set up the room, connect OBS, go live, and stop. Add a video tutorial link at the bottom. Connect the lower stroke of R in the WORLD wordmark.
- Reserve upper-right notifications for outcomes and warnings, dismissing them after five seconds with manual dismissal and scrolling available. Show loading progress in the bottom status bar instead, and use a consistent shortcut layout across pages.
- Center confirmation dialogs over a dimmed, non-interactive page, with Cancel selected by default. Place Start/Stop live before Refresh on the live page.
- Simplify Accounts, Room, OBS, and TTS descriptions and clarify when automatic connection, streaming, and chat listening run, without changing their behavior.
- Move the page name, live status, and OBS status to the upper-right header, with notifications below. Keep only shortcuts and operation progress in the footer, and add the Tab navigation hint to Chat.
- Use text and background color for hover without a full-row underline. Reuse unchanged frames, redraw only the hovered row, and avoid redraws triggered by pointer-shape output to reduce CPU use during rapid mouse movement.
- Reduce idle TUI CPU use: update only the animated separator and reuse the frame when polling finds no visible changes. Pause decoration when the terminal reports focus loss and resume on focus, while keeping background business polling active.
- Keep hover highlighting from repainting a notification that partially covers an action, and preserve the correct pointer shape after asynchronous redraws.
- Fix dark backgrounds appearing across text areas during selection in light themes, preserving the original colors of unselected text and backgrounds.

### Upgrade notes

- This is the first 0.3.0 alpha prerelease, available for early testing and feedback.
- Replace both the main executable and the complete `libexec` directory from the release archive. The new volume setting requires the matching TTS helper; using an older helper causes speech playback to fail.

## [v0.2.1]: 2026-09-20

### Added

- Automatically save login credentials and OBS passwords to `credentials/secrets.json` under the data directory when Linux has no system keyring or cannot connect to the user credential service, and show a file-storage fallback notice. Locked keyrings and denied access still produce errors. The file stores plaintext, with Unix directory and file permissions restricted to `0700` and `0600`, or a restricted ACL on Windows. Profiles with an existing credential file continue using file storage.
- Add CLI options for credential storage (including memory-only mode), proxy, OBS auto-connect/auto-stream, and TTS. Explicit overrides apply only to the current run and are not saved. See `--help` for usage.
- Add `--doctor` read-only diagnostics for configuration, credential services, and optional helpers, without reading secrets or starting graphics, audio, or business services.
- Press `o` on sign-in and identity-verification screens to open the QR code in the system image viewer without interrupting login polling.
- Select multiple overlay displays by name, with mouse or keyboard changes saved and applied immediately. Displays share content snapshots; disconnected selections stay hidden instead of moving to another monitor and return when reconnected.
- Add separate overlay colors for ordinary text, background, Captain, Admiral, Governor, and Super Chat. Preview `#RRGGBB` input live, save on confirmation, or cancel to restore the saved color. The default palette uses pure white text, muted coral guard purchases, warm gold Super Chats, and a dark blue-gray background.

### Changed

- Display and speak guard subscriptions by duration, such as “张三开通了1个月的舰长”, preserving annual and short-term subscriptions. Gifts continue to show quantities.
- Add a thin dark outline to overlay text, enabled by default and toggleable in Overlay content & appearance. Changes are saved and applied immediately. Default text opacity is 1.0 and background opacity is 0.35 for readability over light backgrounds.

### Fixed

- Return to the same appearance option after saving or canceling an edit, preserving the list position for consecutive changes. Press Esc from the appearance list to return to the overlay menu.
- Fix the macOS overlay background covering the entire display. Draw it only within the configured overlay position and size, leaving the rest transparent.
- Fix directory ACL access in the Windows file credential backend while retaining access only for the current user and SYSTEM.
- Fix a race between socket deadlines and the context timer during overlay startup, consistently reporting a deadline error when the startup budget expires.
- Raise the minimum Go version to 1.26.8 so CI and release builds use a toolchain with standard-library security fixes.

### Upgrade notes

- Extract the complete release archive and update both the main executable and the `libexec` directory. Do not mix files from different versions.

## [v0.2.0]: 2026-09-18

This stable release uses the application code from `v0.2.0-rc.2`. All platform packages are rebuilt with the application version set to `0.2.0`. Prereleases remain available for reference.

### Highlights since v0.1.4

- Automatically listen to your live room after sign-in, with support for chat, gifts, Super Chats (SC), and guard events. Browse local history, pause live following, page through messages, and switch rooms.
- Add a native, focus-free, click-through desktop overlay with runtime on/off controls, appearance settings, and independent event selection.
- Add Edge TTS synthesis and MP3 playback. Manage 10 Chinese voices, previews, the playback queue, and spoken events on the dedicated TTS tab.
- Retain history and operation logs for seven days with automatic cleanup, history storage migration, and message deduplication across restarts. Fix log file replacement failures on Windows.
- Confirm before switching or deleting accounts and stop the old broadcast and associated OBS stream. Add controls to restore defaults and clear application data.
- Unify overlay wrapping and overflow behavior on Linux, macOS, and Windows without changing window position or size.
- Bundle the main application, overlay, and speech helper for macOS / Linux amd64 and arm64, plus Windows 386, amd64, and arm64. Include SHA-256 checksums and GitHub build provenance attestations.
- Add Nix Flake builds and development environments. The standalone TUI remains independent of graphics and audio runtimes.

See [v0.2.0-rc.1](https://github.com/TarocchiLive/arcana-world/releases/tag/v0.2.0-rc.1) and [v0.2.0-rc.2](https://github.com/TarocchiLive/arcana-world/releases/tag/v0.2.0-rc.2) for the full changes.

### Upgrade and usage notes

- History databases upgrade automatically. Before downgrading, restore a pre-upgrade database backup rather than opening the upgraded database with an older application version.
- Messages missed while disconnected are not replayed after reconnection. Local history is not a complete financial ledger.
- Overlay content and speech templates currently support Chinese only. Speech synthesis requires an internet connection.
- The Windows desktop overlay is not guaranteed to appear over exclusive-fullscreen applications. Use windowed or borderless-fullscreen mode instead.

## [v0.2.0-rc.2]: 2026-09-18 (prerelease)

### Added

- Add Windows 386 desktop builds, CI checks, and release artifacts. The 32-bit overlay uses standard system calls for argument passing without assembly or CGO.

### Changed

- Move the 10 TTS voice choices from Settings to the TTS tab, alongside playback, preview, queue, and event controls.
- Replace the 64-bit Windows overlay's custom ABI bridges and assembly with typed purego bindings, preserving CGO-free builds and rendering behavior.
- Update Go dependencies, including Bubbles 1.0.0 and Oto 3.5.0.

### Fixed

- Unify overlay wrapping and overflow on Linux, macOS, and Windows. Wrap all events to the available width and show the last complete lines that fit, using a fixed line height from the configured font. Leave spare height blank below the text, keeping newer content visible without partial lines at the top. Preserve the overlay's position, size, and existing height settings.
- Fix operation-log retention cleanup on Windows when replacing an open log file. Writes continue on the same log path after cleanup.

## [v0.2.0-rc.1]: 2026-09-17 (prerelease)

### Added

- Connect Edge TTS synthesis and MP3 playback to a dedicated TTS tab with on/off, preview, and queue-clearing controls. Offer 10 Chinese voices in Settings.
- Give Overlay and TTS separate tabs and independent selections for all 23 default-visible event types, selected by default. Declare Chinese overlay and speech templates in code. Speak only new events and cancel old playback when the account, room, voice, or listening state changes.
- Synthesize and play messages serially, with five waiting slots by default and new messages dropped when full. Support cancelling playback, clearing the queue, and submitting new messages after stopping.
- Add `make tts`, a Nix speech development shell, and a companion package. Include the speech program in `make desktop` and release bundles while keeping the standalone TUI independent of audio runtimes.

- Listen to your room automatically after sign-in. The TUI supports chat, gifts, Super Chats (SC), and guard events, plus local history browsing, pause/follow controls, paging, and room switching.
- Decode 177 registered commands, including newer protobuf gift, interaction, and high-energy ranking messages.
- Separate always-visible, `f`-only, and always-hidden messages in the display allowlist. See the [chat display allowlist](docs/danmaku-whitelist.md) for the full list.
- Show a new-message banner at the top while paused or browsing history. Press `Space` or `End` to return to the latest messages.
- Provide Flake build, application, and check outputs with pinned build dependencies.
- Use `nix develop` (or `nix develop .#arcana-world`) for the independent TUI environment, and `nix develop .#arcana-world-overlay` for overlay development. The Wayland build tag is enabled automatically on Linux. Development uses Go modules without generating `vendor/`.
- Manage the native overlay from its dedicated tab, with runtime on/off controls, saved appearance, and room-status, live-event, or combined content. CLI flags can override the enabled state for one launch.
- Add `make desktop` and a Nix desktop package. Unify cross-platform releases into portable bundles with the main executable at the root and the overlay under `libexec`, while retaining TUI-only development builds.
- Add Restore default settings and Clear application data at the bottom of Settings. Reset restores application preferences, overlay settings, TTS voice, and event selections while keeping OBS configuration and credentials. Clearing requires `arcanaworldclear`, closes background resources, deletes application files and credentials for the current data directory, and exits. Incorrect input returns to Settings.

### Changed

- Complete Chinese and English translations for overlay and TTS controls, event options, voice names, and runtime status. Overlay content and speech templates remain Chinese.
- Enable OBS auto-connect and auto-stream by default for new profiles, preserving explicitly disabled values in existing configurations.
- Move overlay and TTS event selections into submenus. Enter / Space toggles and saves without closing the menu; Esc returns. The two selections remain independent.
- Store each history message's raw business payload once and migrate existing storage automatically, preserving paging, deduplication, and deletion semantics.
- Confirm before switching accounts (including QR sign-in) or deleting the current account, then stop the old broadcast and associated OBS stream. Keep the current account on failure, independently of exit settings.
- Move account and broadcast orchestration from the TUI into the application layer, consistently reclaim idle client connections, and preserve cancellation causes while reading API and cover responses.
- Run basic core tests on Linux, macOS, and Windows. Share one build entrypoint across Make, CI, and six-platform releases while keeping the standalone TUI CGO-free and Nix declarative with the same companion layout.
- Scan default and native speech-tag dependencies weekly. Retain release SHA-256 checksums and add GitHub build provenance attestations. Move the outdated TTS development plan into the local archive.

- Show a yellow notice when cover previews use text characters, recommending a terminal with Kitty graphics support. The notice wraps in narrow windows.
- Default the overlay to the left-center position with zero offsets, font size 16, and live chat as the first content option. Existing saved values remain unchanged.

- Display messages oldest to newest and scroll to the bottom while following. Keep the Other-events toggle fixed at the top.
- Show joins and follows by default, shares and special follows with `f`, and hide mutual-follow notices. All 30 currently supported PK commands are `f`-only.
- Hide cross-room broadcasts, recommendations, offline-room lists, non-PK rankings, and similar notifications even with `f` enabled. Original business payloads are still saved.
- Retain history, raw business payloads, and operation logs for seven days with automatic cleanup. Messages with reliable identifiers support deduplication across restarts.
- Keep OBS connection settings on the dedicated OBS page, with proxy and streaming protocol options on Settings.
- Add interface screenshots, simplify both READMEs, and move the detailed allowlist into `docs/`.
- Separate chat state management from rendering, unify room-switch resets, and remove redundant code and duplicate tests.

### Fixed

- Clear credentials referenced by the active account ID even when the account index is incomplete.
- Close the previous client's idle connections when resetting settings, and close temporary connections used for exit checks.
- Keep wide characters in the cover preview fallback notice within very narrow terminal bounds.
- Locate the overlay under `libexec` relative to the real main executable, so moving the whole bundle or launching through a symlink does not depend on the working directory.
- Remove the trailing colon from Restore appearance defaults and show confirmation and completion messages.
- Fix scrolling so it does not pause live following or delay new messages.
- Preserve reading positions during pauses, paging, and asynchronous reads. Hidden notifications no longer trigger the new-message banner, and unread state does not leak between rooms.
- Fix pausing immediately after returning from a historical room to the live room, which could use the previous room's sequence number.
- Hide the previous chat source immediately on account changes so stale room results cannot reach the overlay.
- Reject protobuf field numbers outside the valid range while continuing to accept valid unknown fields.
- Wrap long status errors to the terminal width and reserve space for their extra lines so error details are not truncated.
- Do not reconnect to unused OBS on exit solely because auto-stream is enabled. Retain stop checks for live auto-stream sessions and known active output, with a shorter connection-failure warning.

### Notes

- History databases upgrade automatically to a versioned format. Before downgrading the application, restore a pre-upgrade database backup rather than opening the upgraded database with an older version.
- Messages missed while disconnected are not replayed after reconnection. Local history may be incomplete and is not a complete financial ledger.
- Messages that can be decoded are not necessarily shown. Unlisted messages and payloads that fail to parse remain hidden.

## [v0.2.0-alpha]: 2026-09-16 (prerelease)

### Added

- Listen to your room automatically after sign-in. The TUI supports chat, gifts, Super Chats (SC), and guard events, plus local history browsing, pause/follow controls, paging, and room switching.
- Decode 177 registered commands, including newer protobuf gift, interaction, and high-energy ranking messages.
- Separate always-visible, `f`-only, and always-hidden messages in the display allowlist. See the [chat display allowlist](docs/danmaku-whitelist.md) for the full list.
- Show a new-message banner at the top while paused or browsing history. Press `Space` or `End` to return to the latest messages.
- Provide Flake build, application, and check outputs with pinned build dependencies.
- Use `nix develop` (or `nix develop .#arcana-world`) for the independent TUI environment, and `nix develop .#arcana-overlay` for overlay development. The Wayland build tag is enabled automatically on Linux. Development uses Go modules without generating `vendor/`.
- Manage the native overlay from Settings, with runtime on/off controls, saved appearance, and room-status, live-chat, or combined content. CLI flags can override the enabled state for one launch.
- Add `make desktop` and a Nix desktop package. Unify cross-platform releases into portable bundles with the main executable at the root and the overlay under `libexec`, while retaining TUI-only development builds.
- Add Restore default settings and Clear application data at the bottom of Settings. Reset affects only Settings-page preferences and keeps OBS configuration and credentials. Clearing requires `arcanaworldclear`, closes background resources, deletes application files and credentials for the current data directory, and exits. Incorrect input returns to Settings.

### Changed

- Show a yellow notice when cover previews use text characters, recommending a terminal with Kitty graphics support. The notice wraps in narrow windows.
- Default the overlay to the left-center position with zero offsets, font size 16, and live chat as the first content option. Existing saved values remain unchanged.

- Display messages oldest to newest and scroll to the bottom while following. Keep the Other-events toggle fixed at the top.
- Show joins and follows by default, shares and special follows with `f`, and hide mutual-follow notices. All 30 currently supported PK commands are `f`-only.
- Hide cross-room broadcasts, recommendations, offline-room lists, non-PK rankings, and similar notifications even with `f` enabled. Original business payloads are still saved.
- Retain history, raw business payloads, and operation logs for seven days with automatic cleanup. Messages with reliable identifiers support deduplication across restarts.
- Keep OBS connection settings on the dedicated OBS page, with proxy and streaming protocol options on Settings.
- Add interface screenshots, simplify both READMEs, and move the detailed allowlist into `docs/`.
- Separate chat state management from rendering, unify room-switch resets, and remove redundant code and duplicate tests.

### Fixed

- Clear credentials referenced by the active account ID even when the account index is incomplete.
- Close the previous client's idle connections when resetting settings, and close temporary connections used for exit checks.
- Keep wide characters in the cover preview fallback notice within very narrow terminal bounds.
- Locate the overlay under `libexec` relative to the real main executable, so moving the whole bundle or launching through a symlink does not depend on the working directory.
- Remove the trailing colon from Restore appearance defaults and show confirmation and completion messages.
- Fix scrolling so it does not pause live following or delay new messages.
- Preserve reading positions during pauses, paging, and asynchronous reads. Hidden notifications no longer trigger the new-message banner, and unread state does not leak between rooms.
- Fix pausing immediately after returning from a historical room to the live room, which could use the previous room's sequence number.
- Hide the previous chat source immediately on account changes so stale room results cannot reach the overlay.
- Reject protobuf field numbers outside the valid range while continuing to accept valid unknown fields.
- Wrap long status errors to the terminal width and reserve space for their extra lines so error details are not truncated.
- Do not reconnect to unused OBS on exit solely because auto-stream is enabled. Retain stop checks for live auto-stream sessions and known active output, with a shorter connection-failure warning.

### Notes

- Messages missed while disconnected are not replayed after reconnection. Local history may be incomplete and is not a complete financial ledger.
- Messages that can be decoded are not necessarily shown. Unlisted messages and payloads that fail to parse remain hidden.

## [v0.1.4]: 2026-09-14

### Added

- Provide Chinese and English interfaces and messages, automatic system-language detection, and manual selection with `--lang`.
- Provide x64 / ARM64 release packages for macOS, Windows, and Linux, covering six platform combinations.
- Provide Chinese and English READMEs, the application icon, installation links, OBS setup instructions, and keyboard documentation.

### Changed

- Separate releases from regular CI and validate version tags before building release packages.
- Include the license and SHA-256 checksums with releases, with workflows for stable and prerelease versions.

## [v0.1.3]: 2026-09-13

### Added

- Provide the terminal broadcast console with QR sign-in, saved-account switching, and broadcast start / stop controls.
- Support editing room titles, categories, announcements, and covers, including cover cropping and terminal previews.
- Support OBS WebSocket v5 connections, writing streaming settings, and automatically synchronizing stream start / stop.
- Support RTMP / SRT and network proxies. Store account credentials and OBS passwords in the system keyring.
- Provide persistent configuration, operation logs, and command-line options including `--config-dir` and `--version`.
- Establish basic tests and CI, with a Linux x64 release archive and checksums.

[v0.3.0-alpha.1]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.3.0-alpha.1
[v0.2.1]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.2.1
[v0.2.0]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.2.0
[v0.2.0-rc.2]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.2.0-rc.2
[v0.2.0-rc.1]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.2.0-rc.1
[v0.2.0-alpha]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.2.0-alpha
[v0.1.4]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.1.4
[v0.1.3]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.1.3
[v0.2.2]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.2.2
[v0.3.0-alpha.2]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.3.0-alpha.2
[v0.3.0-alpha.3]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.3.0-alpha.3
