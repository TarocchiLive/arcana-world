# Changelog

[中文](CHANGELOG.md) · **English**

Versions are listed newest first.

## [v0.4.2]: 2026-10-21

Speech can now use Qwen and Doubao online voices or MeloTTS, Kokoro, and Piper local voices. Edge is built in, and online voices retry on their own when the network hiccups.

### What's changed

- Speech announcements can now use Qwen and Doubao cloud voices, or MeloTTS, Kokoro v1.1, and Piper local voices. Each provider keeps its own settings, and your existing Edge voice carries over.
- Local voices run on the GPU by default, with a CPU option. The first time you pick a model that isn't installed, the app asks whether to download it.
- Download local models in the app, resume a canceled download, or install one manually. A model is only used after it passes verification, and clearing application data keeps it.
- Qwen and Doubao API keys are kept in the credential store, not in the config file.
- Edge, Qwen, and Doubao retry twice on network drops, timeouts, or a busy service; stopping, switching, or quitting cancels them right away.
- Playback supports MP3, WAV, FLAC, and Ogg Vorbis without installing FFmpeg.
- Speech error messages are now translated and tell you what to check.
- Terminal and WebUI help add a "Speech announcements" topic, and the chat overlay help is reorganized.

### Upgrade notes

- The speech helper now bundles the runtime libraries for local voices, so replace both the main program and the `libexec` folder when upgrading.
- Building the full desktop package from source also needs Python 3, CMake, Ninja, and a C/C++ compiler.

## [v0.4.1]: 2026-10-21

Add the RTMPSRT streaming protocol: OBS streams to this computer, and Arcana World forwards it to Bilibili over SRT.

### What's changed

- Settings now offers RTMPSRT as a streaming protocol. It works with OBS on this computer and needs the libsrt 1.5 runtime; see the [README](README.en.md#rtmpsrt-streaming) for installation.
- Before opening the room, the app checks that OBS runs on this computer and libsrt is available, so the room does not go live without a working stream. If Bilibili provides no RTMPSRT URL, the app reports an error.
- After you stop streaming in OBS, you can start again from OBS directly. Forwarding failures are written to Logs, and a failed retry is reported again.
- Help now explains that disconnecting OBS or quitting the app stops an RTMPSRT stream.

### Upgrade notes

- Release packages do not include libsrt; install it as described in the README before using RTMPSRT. Other streaming protocols are unaffected.

## [v0.4.0]: 2026-10-19

Add a browser control panel and styled overlay, with improvements to cover editing, output templates, and streaming diagnostics. This summary covers changes since v0.3.0, including v0.3.1 and v0.3.2.

### What's changed

- Add a local WebUI for accounts, live rooms, OBS, chat, overlays, and TTS, with Chinese and English, light and dark themes, narrow-screen navigation, and a streaming readiness checklist.
- Save the preferred startup interface and open the browser automatically for WebUI; provide a dedicated Windows WebUI launcher and manual checks for the latest stable release.
- Add a styled overlay alongside the native overlay, with text, panel, and animation settings, sample previews without signing in, and macOS Dock / Windows taskbar visibility controls.
- Support separate Chinese and English YAML Go templates for overlays and TTS, reporting invalid configuration files and falling back to defaults; improve message layout and preserve combined emoji and ligatures when truncating text.
- Add cover cropping, zooming, and positioning previews in the terminal and WebUI, including native iTerm2 images; convert uploads to JPEG within the platform's size limits.
- Fix HTTP 403 errors from the Edge TTS endpoint; retry failed synthesis or empty audio up to twice, cancelling immediately on stop or exit.
- Separate detailed diagnostic logs from activity records, recording network, OBS, and helper errors while hiding credentials; support troubleshooting with `--log-level debug`.
- Fix missing RTMP fallback for invalid or unsupported SRT addresses, missed OBS shutdown cleanup after a lost start response, and terminal hangs on interrupt signals.
- Reduce desktop package size and bundle Linux overlay libraries, font configuration, and keyboard layout data; improve Nix builds and deployment and document Scoop and APT installation.

### Upgrade notes

- Extract the entire archive and replace both the main executable and `libexec` to keep helpers matched; Linux and macOS archives now use `.tar.xz`.
- Windows x86 / ARM64 packages provide only the native overlay; Linux overlays require a Wayland desktop with layer-shell support.
- The default WebUI address is `http://127.0.0.1:5273`.

## [v0.4.0-rc.3]: 2026-10-19 (prerelease)

Fix SRT-first mode failing to fall back to RTMP and preventing OBS from starting the stream.

### What's changed

- Fall back to the RTMP address from the same start-live response when the SRT address is unsupported or invalid, validating the fallback address and stream key.
- Simplify the WebUI sidebar by removing the brand icon.

## [v0.4.0-rc.2]: 2026-10-18 (prerelease)

Fix Linux release overlays failing to launch in environments such as Nix, replacing the withdrawn rc.1.

### What's changed

- Bundle Wayland, Pango/Cairo, and other native overlay runtime libraries so the complete bundle can be relocated and launched directly.
- Include XKB keyboard layout data to fix Elegant overlay crashes when the expected system data directory is missing.
- Use bundled font configuration with system and user fonts for both overlays, avoiding incompatible host Fontconfig rules while preserving explicit environment overrides.

### Upgrading

Replace the main executable and the entire `libexec` directory, not just the overlay executables; Linux still requires a Wayland desktop with layer-shell support.

## [v0.4.0-rc.1]: 2026-10-18 (prerelease)

This release candidate redesigns the WebUI, reduces Elegant overlay bundle sizes, and improves diagnostic logging and OBS shutdown cleanup.

### What's changed

- Redesign the WebUI with sidebar navigation, global live and OBS status, a go-live checklist, and unified management dialogs, with bottom navigation on narrow screens.
- View users by clicking chat names, see new-message prompts while scrolling back, and automatically save number and color settings after adjustments.
- Preview overlay samples from the WebUI Outputs page; actual content returns when preview is disabled or every page leaves Outputs, without adding samples to chat history or speech.
- Trim Qt runtimes and third-party notice files on Windows, Linux, and macOS while retaining font shaping and both Native and Elegant overlays; use `.tar.xz` on Linux and macOS and stronger ZIP compression on Windows.
- Separate diagnostic logs from on-screen activity records, recording network, OBS, and helper-process errors while hiding sensitive credentials; use `--log-level debug` for more detail.
- Fix shutdown leaving OBS streaming when a start takes effect but its response is lost through disconnection or cancellation.
- Fix Elegant overlay builds and runtime deployment under Nix; rebuilds remove stale Qt files and preserve the existing runtime if dependency collection fails.
- Simplify speech error messages by removing repeated prefixes.

### Upgrading

Extract the entire release archive and replace both the main executable and `libexec`; Linux and macOS archives now use the `.tar.xz` extension.

## [v0.4.0-alpha.1]: 2026-10-14 (prerelease)

### Added

- Add a local Web control panel for accounts, live streaming, OBS, chat and moderation, overlay settings, and TTS settings. It includes Chinese/English UI and light, dark, and system themes, and defaults to `http://127.0.0.1:5273`.
- Choose and save the default interface (TUI / WebUI). WebUI opens the browser on startup; Windows packages also include a WebUI launcher.
- Add Elegant overlay with text, panel, and motion settings alongside Native overlay. It supports Wayland layer-shell and Dock/taskbar visibility controls on macOS and Windows.
- Preview overlay appearance and motion for different chat events with sample messages, without signing in.
- Check for the latest stable release manually and open its release page.

### Changed

- Preview, zoom, and pan cover crops in the terminal and Web panel; terminal cover previews now support native iTerm2 images. Upload covers as 720×540 JPEG files up to 256 KiB to avoid oversized-image rejections.
- Group overlay appearance settings and support arrow-key adjustments with live preview, save, and cancel. Prefer complete recent messages when display space is limited.
- Retry failed TTS synthesis or empty audio up to twice, canceling immediately on stop or exit.
- Document Scoop installation on Windows and APT installation on Debian/Ubuntu, and include startup guidance in Windows packages.

### Fixed

- Fix the terminal interface sometimes hanging when interrupted.
- Fix overlay and TTS text cleanup and truncation splitting joined emoji or script shaping.

### Upgrade notes

- Replace the main executable and the entire `libexec` directory together to avoid mixing old overlay helpers. Windows 386/arm64 packages currently provide only Native overlay; Elegant overlay on Wayland requires compositor support for layer-shell.

## [v0.3.2]: 2026-10-12

### Fixed

- Fix Edge TTS handshake failures caused by HTTP 403 from the legacy synthesis endpoint; migrate to Microsoft's new endpoint and use its hostname for proxy bypass rules.

## [v0.3.1]: 2026-10-09

### Added

- Customize overlay and TTS text independently with Go templates in YAML files. Chinese and English configurations are separate; missing defaults are generated at startup, and edits take effect after restarting.
- Template errors identify the file and cause, then fall back to built-in defaults for that file during the current session. Warnings remain until dismissed, and existing configuration files are never overwritten.
- Add the in-app “Customize overlay and TTS formats” article with fields, examples, recovery instructions, and the Go template documentation link. TUI chat formatting remains unchanged.

## [v0.3.0]: 2026-09-29

### Added

- Add a wrapping chat input with a 40-character limit and retain drafts after failed sends. Select text in chat, history, logs, and notifications by dragging, then press `y` to copy.
- Add viewer rankings and Fleet lists showing the top 20 entries. Show the viewer count beside the live status and refresh it every 30 seconds.
- Add room-admin, live-blocklist, and separate mute management with username or UID lookup and identity confirmation. Mute for a chosen number of hours or permanently; click a chat username to open a profile and moderation actions.
- Save TTS volume from 0 to 100. Choose from five themes and configure animation, compact headers, and notifications.
- Support mouse clicks, scrolling, and hover feedback, with exit confirmation enabled by default. Organize in-app help by topic.

### Changed

- Upgrade to Charm v2 and redesign TUI pages, forms, and pickers. Improve Chinese input cursor positioning and IME candidate placement.
- Load the latest 10 messages from the past 24 hours at startup and retain up to 30 by default in the main chat view. Query history on a separate page while preserving drafts and reading position.
- Share the chat snapshot between the overlay and TUI so new messages, retractions, and expired entries stay in sync. Reduce idle and mouse-movement redraw work.
- Render profile images natively in Kitty/Ghostty and use character previews elsewhere. Keep public UIDs visible in logs while hiding login credentials and stream keys.

### Fixed

- Stop showing an empty-list warning when the automatic refresh after unmuting, dismissing an admin, or unblocking a user returns no entries. Explicit queries and API errors still show feedback.
- Save the request device identifier in `device.db` inside the data directory. Instances sharing that directory reuse it across restarts and client rebuilds. Resetting settings preserves it; clearing application data removes it.
- Remove the option to ignore the current account in viewer counts and lists. Display the API's viewer count without subtraction and keep the current account in the returned list.
- Fix Windows 32-bit borders and layout, username click targets after live updates, and stale viewer-count responses replacing newer results. Reject self-mutes and explain the checks needed when the platform refuses a moderation action.

### Upgrade notes

- Replace both the main executable and the complete `libexec` directory. Do not mix overlay or TTS helpers from different versions.
- Back up the data directory before upgrading. History databases upgraded by 0.3.0 cannot be used directly with 0.2.x.

## [v0.3.0-rc.1]: 2026-09-28 (prerelease)

### Changed

- Use the live blocklist APIs to block users, list blocked users, and unblock them. Confirmation explains that blocking removes relationships and restricts interactions.
- Mute users for a chosen positive whole number of hours or permanently, with the duration shown before confirmation. Add separate mute management (`s`) to Chat for listing and unmuting users; `b` still opens the blocklist.
- Move application configuration into a separate package so domain types, Bilibili APIs, and OBS no longer depend indirectly on overlay or TTS code. Keep the configuration file format and defaults unchanged.

### Fixed

- Reject attempts to mute the signed-in account with a clear explanation. For mute API code `100004`, suggest checking the target and moderation permissions while retaining the code and keeping raw responses private.
- Keep public UIDs visible in logs and status messages so operation targets can be identified. Login credentials and stream keys remain hidden.

### Upgrade notes

- Replace both the main executable and the complete `libexec` directory. Back up the data directory before upgrading; history databases upgraded by 0.3.0 cannot be used directly with 0.2.x.

## [v0.3.0-alpha.4]: 2026-09-25 (prerelease)

### Added

- Add “Ignore myself in viewer list” to Settings, enabled by default. Viewer counts exclude the current account, and viewer details hide it. Turning this off restores the original counts and list; Fleet is unchanged.

### Fixed

- Share the same data and refresh results between the overlay and the TUI main chat list, without loading extra history. Startup messages, new arrivals, Super Chat withdrawals, and removal of older entries now update together. Keep independent overlay event filtering and the six-event display limit.

### Upgrade notes

- Replace both the main executable and the complete `libexec` directory. Back up the data directory before upgrading; history databases upgraded by 0.3.0 alpha cannot be used directly with 0.2.x.

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
[v0.3.0-alpha.4]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.3.0-alpha.4
[v0.3.0-rc.1]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.3.0-rc.1
[v0.3.0]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.3.0
[v0.3.1]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.3.1
[v0.3.2]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.3.2
[v0.4.0-alpha.1]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.4.0-alpha.1
[v0.4.0-rc.1]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.4.0-rc.1
[v0.4.0-rc.2]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.4.0-rc.2
[v0.4.0-rc.3]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.4.0-rc.3
[v0.4.0]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.4.0
[v0.4.1]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.4.1
[v0.4.2]: https://github.com/TarocchiLive/arcana-world/releases/tag/v0.4.2
