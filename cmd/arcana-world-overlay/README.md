# Arcana World Overlay

**中文** · [English](README.en.md)

Arcana World 的跨平台桌面文字浮层会在固定屏幕区域显示文字，不抢焦点，也支持鼠标穿透。它可以独立显示提示、时钟或持续更新的文字，也可以随 Arcana World 启动，显示直播间状态和实时弹幕。

[返回 Arcana World](../../README.md)

## 功能

- 文字动态更新，支持中文和自动换行。
- 九宫格定位，可调整位置、大小和四边留白。
- 可选字体、字号、字重和斜体。
- 文字和背景分别调整透明度，也可以只显示文字。
- 支持选择显示器；切换前台应用不会改变浮层位置。

## 支持的平台

| 平台 | 支持情况 |
| --- | --- |
| macOS | 支持，可跨桌面空间显示 |
| Windows | 支持 Windows 10 1803 及以上版本 |
| Linux | 支持 KDE Plasma、niri 等 Wayland 桌面 |
| GNOME | GNOME Mutter 缺少 layer-shell，暂不兼容 |
| X11 桌面 | 暂无支持计划 |

不保证显示在锁屏、系统安全界面或游戏独占全屏之上。普通 macOS 显示不需要辅助功能或屏幕录制权限。

## 常用操作

### 调整位置和外观

默认显示在屏幕左侧、垂直居中，横纵偏移均为 0，大小为 420 × 180，字号为 16。位置和字号使用逻辑单位，随系统显示缩放（DPI）调整。

```sh
./bin/libexec/arcana-world-overlay -text '欢迎来到直播间' -anchor bottom-left -x 24 -y 24 -width 480 -height 200 -padding 20 -font-size 28 -background-alpha 0.25
```

| 参数 | 用途 |
| --- | --- |
| `-text '文字'` | 要显示的内容 |
| `-anchor left` | 选择屏幕上的位置，见下方九宫格 |
| `-x 0 -y 0` | 横向、纵向偏移；靠边时正数向屏幕内侧移动，居中时正数向右、向下移动 |
| `-width 420 -height 180` | 浮层宽高，必须大于 0 |
| `-padding 12,16,12,16` | 按上、右、下、左设置留白；一个数表示四边相同 |
| `-font '字体名称'` | 选择字体，不设置则使用系统默认字体 |
| `-font-size 16 -font-weight 500` | 字号和字重；字重范围为 100–900 |
| `-italic` | 使用斜体 |
| `-text-alpha 0.9` | 文字不透明度，0 为完全透明，1 为不透明 |
| `-background-alpha 0.3` | 背景不透明度；设为 0 则只显示文字 |

位置名称：

| 左 | 中 | 右 |
| --- | --- | --- |
| `top-left` | `top` | `top-right` |
| `left` | `center` | `right` |
| `bottom-left` | `bottom` | `bottom-right` |

位置、大小和留白使用随屏幕缩放的逻辑单位。所有文字按可用宽度自动换行，行高由配置字体确定；超出高度时只显示末尾能容纳的完整行，可以从一条消息或用户名的中间开始。文字顶部对齐，不足一行的剩余高度留在底部；内容区域连一行都放不下时不显示文字。增加高度或减小字号可显示更多行，文字更新不会改变浮层位置或尺寸。

### 动态更新

显示每秒更新的时钟：

```sh
./bin/libexec/arcana-world-overlay -text '当前时间' -clock
```

通过管道接收其他程序输出的文字：

```sh
printf '%s\n' '新的提示内容' | ./bin/libexec/arcana-world-overlay -stdin
```

`-stdin` 每收到一行就替换全部文字，输入结束后保留最后一行，不会自动关闭。单行最多约 1 MiB。它不能与 `-clock` 同时使用。

### 选择显示器

程序默认使用启动时选中的显示器，也可手动指定：

- macOS：`-display <显示器 ID>`。
- Windows：`-display 2` 选择第二个显示器，或用 `-output <设备名>`。
- Linux：`-output DP-1` 等桌面中的实际输出名称。

`-display` 和 `-output` 不要同时使用。

在主程序的「弹幕浮层 → 选择浮层显示器」中，可按名称独立勾选多个显示器。点击条目或按 `Enter` / `Space` 后立即保存并生效，无需重启浮层；按 `r` 刷新显示器列表。全部取消勾选会隐藏所有浮层。已选显示器断开后不转移到其他屏幕，重新连接后恢复显示。

独立运行时，`-list-displays` 输出显示器名称和稳定 ID 的 JSON 列表；使用 `-displays '["ID1","ID2"]'` 在多个显示器上显示相同内容。此参数不能与 `-display` 或 `-output` 同时使用。

### 随 Arcana World 启动

完整解压便携包后，运行根目录的 `arcana-world`（Windows 为 `arcana-world.exe`），在「弹幕浮层」页开启「原生浮层」，无需手工启动 helper。保留根目录主程序、`LICENSE` 和 `libexec/arcana-world-overlay`（Windows 带 `.exe`）的布局，搬迁时整体移动目录。本地使用 `make desktop` 构建后，也可通过命令行启用：

```sh
./bin/arcana-world -overlay
```

默认显示实时事件，并在主程序退出时关闭；尚未登录或没有事件时内容为空。在主程序的「弹幕浮层」页可开关「原生浮层」，通过「浮层内容与外观」调整内容模式、位置、尺寸、留白、字体和透明度，通过「选择浮层显示器」选择显示屏幕。设置会保存，下次启动继续使用；浮层启动失败不影响其他功能，浮层页和日志页会显示原因。失败后关闭再启用可重试。

内容模式依次为「实时弹幕」「房间状态」「状态与弹幕」。弹幕模式使用当前登录账号的直播间，最多保留六条勾选的事件，实际显示范围取决于换行后的行数和可用高度；默认包含弹幕页无需开启 `f` 即可显示的全部事件，模板固定使用中文。事件选择与 TTS 独立，不受 TUI 弹幕页的历史翻阅、房间选择或当前页面影响；关闭弹幕监听后清空浮层事件。每次刷新最多扫描最近 256 条新记录，密集通知期间可能跳过较早的事件，完整记录仍可在弹幕页查看。

`-overlay` 和 `-overlay=false` 只覆盖本次启动的开关，不自动改写保存的设置；在 TUI 中操作开关会保存。使用 `-overlay-executable /path/to/arcana-world-overlay` 指定其他位置的浮层程序。主程序管理模式下，上面的独立浮层外观参数不生效。

已有保存值不会因默认值更新而改变。「恢复默认设置」会重置应用偏好、浮层、TTS 音色和事件选择，但保留 OBS 配置、密码和 B 站登录信息。「恢复默认外观」需确认后执行，仅重置浮层外观，不改变内容模式和开关；完成后显示恢复成功提示。

## 构建与启动

`arcana-world-<goos>-<goarch>.tar.gz` / `.zip` 便携包包含主程序和 `libexec` 目录下的浮层程序。源码构建命令均在**项目根目录**执行，并先安装项目要求版本的 Go。

macOS 还需安装 Xcode Command Line Tools；Linux 需安装 Wayland、Pango/Cairo 开发库和 `pkg-config`。Debian/Ubuntu 可使用：

```sh
sudo apt-get install libwayland-dev libpango1.0-dev libcairo2-dev pkg-config
```

macOS / Linux：

```sh
make overlay
./bin/libexec/arcana-world-overlay -text '欢迎来到直播间'
```

使用 `make desktop` 同时构建 `bin/arcana-world` 与 `bin/libexec/arcana-world-overlay`（Windows 带 `.exe`），`make build` 仍只构建 TUI，无需图形开发库。Nix 默认包包含浮层，可运行 `nix run -- -overlay`；`nix run .#arcana-world` 仅运行 TUI。

Windows（PowerShell）：

```powershell
go build -o bin/libexec/arcana-world-overlay.exe ./cmd/arcana-world-overlay
.\bin\libexec\arcana-world-overlay.exe -text '欢迎来到直播间'
```

在启动它的终端按 `Ctrl+C` 关闭。加上 `-duration 30s` 可在 30 秒后自动关闭。上面的独立调用命令使用本地构建路径；便携包中使用 `./libexec/arcana-world-overlay`，Windows 对应 `.\libexec\arcana-world-overlay.exe`。

## 更多选项

```sh
./bin/libexec/arcana-world-overlay -help
```

帮助中的 `-socket` 由主程序管理浮层时使用，普通运行无需设置。
