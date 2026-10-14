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
| X11 桌面 | 精美浮层通过 xcb 插件支持；原生浮层不支持 |

不保证显示在锁屏、系统安全界面或游戏独占全屏之上。普通 macOS 显示不需要辅助功能或屏幕录制权限。

## 常用操作

### 调整位置和外观

默认显示在屏幕左侧、垂直居中，横纵偏移均为 0，大小为 420 × 180，字号为 16。位置和字号使用逻辑单位，随系统显示缩放（DPI）调整。

```sh
./bin/libexec/arcana-world-overlay-native -text '欢迎来到直播间' -anchor bottom-left -x 24 -y 24 -width 480 -height 200 -padding 20 -font-size 28 -background-alpha 0.25
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
| `-outline=false` | 关闭文字描边；默认开启 |
| `-text-alpha 1` | 文字不透明度，0 为完全透明，1 为不透明 |
| `-background-alpha 0.35` | 背景不透明度；设为 0 则只显示文字 |

位置名称：

| 左 | 中 | 右 |
| --- | --- | --- |
| `top-left` | `top` | `top-right` |
| `left` | `center` | `right` |
| `bottom-left` | `bottom` | `bottom-right` |

位置、大小和留白使用随屏幕缩放的逻辑单位。所有文字按可用宽度自动换行，行高由配置字体确定。超出高度时优先从末尾能完整显示的消息开头绘制，消息以换行分隔；若可见范围内没有非空消息的开头，则显示长消息末尾能容纳的完整行。文字顶部对齐，剩余高度留在底部；内容区域连一行都放不下时不显示文字。增加高度或减小字号可显示更多行，文字更新不会改变浮层位置或尺寸。

### 动态更新

显示每秒更新的时钟：

```sh
./bin/libexec/arcana-world-overlay-native -text '当前时间' -clock
```

通过管道接收其他程序输出的文字：

```sh
printf '%s\n' '新的提示内容' | ./bin/libexec/arcana-world-overlay-native -stdin
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

完整解压便携包后，运行根目录的 `arcana-world`（Windows 为 `arcana-world.exe`），在「弹幕浮层」页选择「原生浮层」或「精美浮层」并启用，无需手工启动 helper。保留根目录主程序、`LICENSE` 和完整 `libexec` 目录，搬迁时整体移动。本地使用 `make desktop` 构建后，也可通过命令行启用：

```sh
./bin/arcana-world -overlay
```

默认显示实时事件，并在主程序退出时关闭；主弹幕列表尚未加载或没有可显示事件时内容为空。在「弹幕浮层」页启用所选后端，通过「浮层内容与外观」调整内容模式、位置、尺寸、留白、字体和透明度，通过「选择浮层显示器」选择显示屏幕。设置会保存，下次启动继续使用；浮层启动失败不影响其他功能，浮层页和日志页会显示原因。失败后关闭再启用可重试。

「浮层内容与外观」按所选样式显示设置，并保留各自偏好。原生浮层首页保留内容模式和恢复默认外观，其他选项分为「布局」「文字」「颜色与透明度」；精美浮层提供文字、面板与动效设置。Esc 逐级返回；保存或取消编辑后回到原字段。本次打开设置期间，各分组分别记住选中位置。

无弹幕时，可在「弹幕浮层」页开启「示例预览」检查换行、字体、颜色、尺寸和动效，无需登录。请先启用所选浮层并等待就绪。示例每 1.5 秒滚入下一条，循环展示普通弹幕、舰长／提督／总督弹幕、醒目留言、礼物、连击送礼、上舰、进场和点赞，每行均标明“示例”，随界面语言显示；不写入历史、不触发 TTS，也不保存预览开关。关闭预览、离开浮层页或关闭浮层后恢复实际内容；预览期间收到的新事件会在恢复后正常显示。

浮层运行时，外观输入框中的有效值会即时预览，包括位置偏移、尺寸、留白、字体、颜色和透明度。Enter 保存，Esc 恢复已保存值；无效输入保留最近一次有效预览。预览本身不写入配置，可配合示例预览调整外观。

数值输入框支持 ↑/↓ 增减：位置、尺寸、留白和字号的步长为 1，字重为 100，透明度为 0.01（保留两位小数）。当前值无效或调整后越界时保持不变。左右键仍移动光标，也可直接输入数值；步进后仍需 Enter 保存，Esc 撤销。

内容模式依次为「实时弹幕」「房间状态」「状态与弹幕」。弹幕模式与 TUI 主弹幕列表共用数据和刷新结果，不单独补读历史；从主列表中显示最新六条勾选的事件，实际显示范围取决于换行后的行数和可用高度。启动时沿用主列表近 24 小时内最多 10 条的加载范围，后续消息、撤回和列表淘汰同步更新。模板按界面语言选择配置目录中的 `templates/overlay.zh-CN.yaml` 或 `templates/overlay.en.yaml`，可编辑对应的外部 YAML 文件定制，保存后重启生效。浮层事件选择与 TTS 独立；翻阅历史或切换页面不会改变浮层的数据来源，关闭弹幕监听后清空浮层事件。

`-overlay` 和 `-overlay=false` 只覆盖本次启动的开关，不自动改写保存的设置；在 TUI 中操作开关会保存。使用 `-overlay-executable /path/to/arcana-world-overlay-native` 指定其他位置的浮层程序，必须与所选后端一致。主程序管理模式下，上面的独立浮层外观参数不生效。

自定义外观会保留；精美浮层的旧默认文字样式会自动迁移为统一字号、关闭最新消息渐变高亮及 95% 历史文字不透明度，事件颜色不变。「恢复默认设置」会重置应用偏好、浮层、TTS 音色和事件选择，但保留 OBS 配置、密码和 B 站登录信息。「恢复默认外观」需确认后执行，仅重置浮层外观，不改变内容模式和开关；完成后显示恢复成功提示。

## 构建与启动

`arcana-world-<goos>-<goarch>.tar.gz` / `.zip` 便携包包含主程序和 `libexec` 目录下的浮层程序。源码构建命令均在**项目根目录**执行，并先安装项目要求版本的 Go。

原生浮层在 macOS 上还需安装 Xcode Command Line Tools；Linux 需安装 Wayland、Pango/Cairo 开发库和 `pkg-config`。完整桌面构建还需下方[精美浮层构建与运行库](#精美浮层构建与运行库)列出的依赖。Debian/Ubuntu 的原生浮层依赖可使用：

```sh
sudo apt-get install libwayland-dev libpango1.0-dev libcairo2-dev pkg-config
```

macOS / Linux：

```sh
make overlay
./bin/libexec/arcana-world-overlay-native -text '欢迎来到直播间'
```

`make desktop` 构建 TUI、两种浮层 helper、语音 helper 及图形运行库；`make desktop-native` 明确只包含原生浮层。`make build` 仍只构建不依赖 CGO 的 TUI。Nix 默认构建完整桌面包，`nix run .#arcana-world-desktop-native -- -overlay` 可选择原生包；macOS 完整包要求 nixpkgs 提供 Qt >= 6.11，版本不足会明确报错。

Windows（PowerShell）：

```powershell
go build -o bin/libexec/arcana-world-overlay-native.exe ./cmd/arcana-world-overlay
.\bin\libexec\arcana-world-overlay-native.exe -text '欢迎来到直播间'
```

在启动它的终端按 `Ctrl+C` 关闭。加上 `-duration 30s` 可在 30 秒后自动关闭。便携包中的独立调用路径以 `./libexec/` 开头，本地构建路径以 `./bin/libexec/` 开头。

## 更多选项

```sh
./bin/libexec/arcana-world-overlay-native -help
```

帮助中的 `-socket` 由主程序管理浮层时使用，普通运行无需设置。

## 精美浮层构建与运行库

精美浮层内部使用 Qt Widgets，对应 `libexec/arcana-world-overlay-qt`；原生浮层对应 `libexec/arcana-world-overlay-native`，Windows 均加 `.exe`。两种 helper 共用 v4 协议，拒绝与主程序选择不符的后端。用户只需启动 `arcana-world`。

macOS amd64/arm64、Linux amd64/arm64 和 Windows amd64 完整发行包包含两种后端。Windows 386、arm64 保留原生包：当前发行工具链没有对应的 Qt6 MinGW 构建。原生包中选择精美浮层会明确报告组件缺失，不会自动切换成原生浮层。

在目标系统和架构上安装 CMake、Python 3、C++17 编译器以及匹配的 Qt 开发库：

- macOS：安装 Xcode Command Line Tools，执行 `brew install qt cmake`。稳定原生显示器 ID 要求 Qt >= 6.11。`CMAKE_PREFIX_PATH` 指向 `brew --prefix qt`，`QTPATHS` 指向该目录的 `bin/qtpaths`。
- Linux：安装 Qt >= 6.5（Core/Gui/Widgets/WaylandClient）、LayerShellQt >= 6.3、Wayland 开发库和 `patchelf`。发行脚本在 Ubuntu 24.04 上构建并缓存 Qt 6.8.3、LayerShellQt 6.3.6，保持原有 glibc 基线。`scripts/setup-linux-qt.sh` 使用 sudo 安装构建依赖；非 CI 环境还需将安装前缀设为 `CMAKE_PREFIX_PATH`，将其 `bin` 加入 PATH，并将 `QTPATHS` 指向 `bin/qtpaths`。
- Windows amd64：使用 MSYS2 UCRT64 的 Qt6、GCC、CMake、Ninja，以及原生 Windows Go 和 Python。Qt 与 cgo 必须使用同一套 UCRT64 编译器，不可混用 MSVC Qt。设置 `CMAKE_GENERATOR=Ninja`、`CC=gcc`、`CXX=g++`，`QTPATHS` 指向 UCRT64 的 `qtpaths6.exe`。

`make overlay-qt` 构建并部署精美浮层 helper。部署脚本将 Qt 插件及间接动态库依赖复制到 `libexec`，改写 macOS/Linux 加载路径，并对修改后的 macOS 二进制重新临时签名。Windows 使用 `windeployqt`，随后补齐 MinGW 依赖。Linux 包含 LayerShellQt 和 Wayland 客户端依赖，但使用系统的 glibc、内核、显示服务和显卡驱动。移动或打包时必须保留整个目录。

macOS helper 位于私有 `Arcana World Elegant Overlay.app/Contents/MacOS` 中，外层 helper 名称是相对符号链接。主程序在启动及枚举显示器前解析真实路径，使系统读取应用包中的产品名和初始后台策略；运行中仍可切换 Dock 显示。独立调试时应直接运行 `./bin/libexec/Arcana World Elegant Overlay.app/Contents/MacOS/arcana-world-overlay-qt`（路径需加引号），避免通过符号链接启动丢失应用身份。

Wayland 下精美浮层使用真正的 LayerShellQt surface，要求合成器提供 layer-shell。GNOME/Mutter 不受支持，不会回退为普通窗口。原生浮层在 Linux 上只支持 Wayland；精美浮层还可通过 xcb 插件运行于 X11。

独立演示：`./bin/libexec/arcana-world-overlay-qt -demo -duration 30s`。`-demo`、`-clock`、`-stdin` 互斥。`-unlocked` 允许拖动，`-reduced-motion` 关闭动效；两项仅用于独立精美浮层，不能与受管理的 `-socket` 模式组合。精美浮层不创建菜单栏／托盘图标或右键菜单。

精美外观参数不带后端品牌前缀：`-align`、`-history-scale`、`-latest-scale`、`-line-spacing`、`-message-spacing`、`-history-alpha`、`-max-messages`、`-max-lines`、`-gradient`、`-accent-start`、`-accent-end`、`-radius`、`-border-alpha`、`-shadow`、`-animation`、`-animation-ms`、`-animation-distance`。精美 helper 的 `-help` 列出取值范围，主程序管理模式使用保存设置。

「弹幕浮层」页的第二项紧跟启用开关，用于选择原生浮层或精美浮层，并说明两者的外观与设置差异。精美浮层的外观设置在 macOS 上提供「在 Dock 中显示」，在 Windows 上提供「在任务栏中显示」。该偏好保存为 `overlay.qt.show_in_taskbar`（默认 `false`），通过 `config.qt.show_in_taskbar` 传给 helper，保存后即时生效。原生浮层不使用此偏好。Linux 的 X11 和 Wayland 均不启用此功能，但保留已保存的值；即使保存为 `true`，Wayland 仍使用 layer-shell。

独立运行精美 helper 时可用 `--show-in-taskbar` 启用此设置。原生 helper 拒绝该参数，包括 `--show-in-taskbar=false`；受管理的 `-socket` 模式不接受独立运行参数。Dock／任务栏显示可帮助 OBS 查找应用或窗口，但能否捕获仍取决于权限和捕获方式，不会因此创建托盘图标。
