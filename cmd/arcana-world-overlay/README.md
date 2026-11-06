# Arcana World Overlay

**中文** · [English](README.en.md)

桌面浮层在屏幕固定区域显示直播间状态和实时弹幕。浮层半透明、不抢焦点，鼠标可以穿透。它也能单独运行，显示提示、时钟或其他程序输出的文字。

[返回 Arcana World](../../README.md)

## 两种浮层

| 浮层 | 特点 |
| --- | --- |
| 原生浮层 | 简洁的文字展示，可调整字体、颜色、描边和透明度 |
| 精美浮层 | 增加消息入场动画、渐变、圆角和阴影，排版和动效可细调，可在无新消息时自动隐藏 |

两种浮层显示相同的内容，外观设置分别保存。

## 支持的平台

| 平台 | 原生浮层 | 精美浮层 |
| --- | --- | --- |
| macOS（Apple 芯片） | 支持 | 支持 |
| Windows x64（10 1803 及以上） | 支持 | 支持 |
| Windows ARM64（10 1803 及以上） | 支持 | 不支持 |
| Linux Wayland（KDE Plasma、niri 等提供 layer-shell 的桌面） | 支持 | 支持 |
| GNOME Wayland | 不支持 | 需安装扩展，见 [GNOME 精美浮层](../../docs/gnome-overlay.md) |
| Linux X11 | 不支持 | 不支持 |

macOS 上浮层跨桌面空间显示，不需要辅助功能或屏幕录制权限。各平台均不保证显示在锁屏、系统安全界面或独占全屏的游戏之上。

## 开启浮层

1. 完整解压发行包，运行根目录的 `arcana-world`（Windows 为 `arcana-world.exe`）。
2. 打开「弹幕浮层」页，在「浮层样式」中选择原生浮层或精美浮层，然后打开「浮层」开关。
3. 在「选择浮层显示器」中勾选显示器，在「选择浮层显示事件」中勾选要显示的事件。

WebUI 在「输出」中提供相同的设置。浮层显示在运行 Arcana World 的电脑上，随主程序退出而关闭。主程序、`LICENSE` 和 `libexec` 目录需放在一起，搬迁时整体移动。

命令行 `arcana-world -overlay` 或 `-overlay=false` 只影响本次启动，不改变保存的开关。

### 显示内容

「浮层内容与外观」中的「内容模式」可选「实时弹幕」「房间状态」「状态与弹幕」。

弹幕与主弹幕列表同步，显示其中最新的六条已勾选事件，实际行数取决于换行和浮层高度。浮层的事件选择与语音播报互不影响；关闭弹幕监听会清空浮层。修改显示文字的格式见应用内帮助「自定义浮层与 TTS 播报格式」。

### 调整外观

在「浮层内容与外观」中调整位置、尺寸、字体、颜色和透明度；精美浮层另有「精美浮层文字」「精美浮层面板」「精美浮层动效」三组设置。

- 浮层运行时，输入的有效值立即显示在浮层上。Enter 保存，Esc 恢复已保存的值。
- 数值输入框可用 ↑/↓ 步进：位置、尺寸、留白和字号每次 1，字重每次 100，透明度每次 0.01。
- 没有弹幕时，在「弹幕浮层」页打开「示例预览」，无需登录即可查看各类事件的显示效果。示例不写入历史，也不播报；关闭预览或离开该页后恢复实际内容。
- 精美浮层开启「无新消息时自动隐藏」后，内容在设定秒数内没有变化时面板渐隐，新消息到达时渐显。
- 精美浮层在 macOS 可开启「在 Dock 中显示」，在 Windows 可开启「在任务栏中显示」，便于 OBS 找到窗口。能否捕获仍取决于系统权限和捕获方式。

「恢复默认外观」只重置浮层外观，不改变内容模式和开关。

### 选择显示器

「选择浮层显示器」可勾选多个显示器，选择后立即生效，按 `r` 刷新列表。全部取消会隐藏浮层。已选的显示器断开后，浮层不会转移到其他屏幕，重新连接后恢复显示。

### 看不到浮层

- 「弹幕浮层」页显示浮层状态和最近的错误，详细原因见「日志」页。失败后关闭再打开「浮层」开关即可重试。
- 确认已选显示器、弹幕监听已开启、对应事件已勾选，且消息没有被屏蔽词隐藏。
- 开启自动隐藏时，没有新消息的面板会隐藏。

## 单独运行

浮层程序位于发行包的 `libexec` 目录，本地构建位于 `bin/libexec`。Windows 程序名加 `.exe`。

| 浮层 | 程序 |
| --- | --- |
| 原生浮层 | `libexec/arcana-world-overlay-native` |
| 精美浮层（Linux、Windows） | `libexec/arcana-world-overlay-qt` |
| 精美浮层（macOS） | `libexec/Arcana World Elegant Overlay.app/Contents/MacOS/arcana-world-overlay-qt` |

在终端按 `Ctrl+C` 关闭，或加 `-duration 30s` 在 30 秒后自动关闭。`-help` 列出全部参数和取值范围。由主程序管理浮层时，本节参数不生效。

### 位置和外观

```sh
./libexec/arcana-world-overlay-native -text '欢迎来到直播间' -anchor bottom-left -x 24 -y 24 -width 480 -height 200 -padding 20 -font-size 28 -background-alpha 0.25
```

| 参数 | 用途 |
| --- | --- |
| `-text` | 显示的文字 |
| `-anchor` | 屏幕锚点，见下表 |
| `-x`、`-y` | 偏移；贴边时正数向屏幕内侧，居中时正数向右、向下 |
| `-width`、`-height` | 宽度和高度 |
| `-padding` | 留白；一个数用于四边，或按上、右、下、左写四个数 |
| `-font` | 字体名称，不设置时使用系统默认字体 |
| `-font-size`、`-font-weight` | 字号；字重 100–900 |
| `-italic` | 斜体 |
| `-outline=false` | 关闭文字描边 |
| `-text-alpha`、`-background-alpha` | 文字和背景的不透明度，0–1；背景设为 0 时只显示文字 |

| 左 | 中 | 右 |
| --- | --- | --- |
| `top-left` | `top` | `top-right` |
| `left` | `center` | `right` |
| `bottom-left` | `bottom` | `bottom-right` |

默认位于屏幕左侧、垂直居中，大小 420 × 180，字号 16。位置、尺寸、留白和字号使用逻辑单位，随系统缩放调整。

文字顶部对齐并自动换行。放不下时优先显示末尾的完整消息，消息以换行分隔。文字更新不会改变浮层的位置和尺寸。

### 动态文字

```sh
# 每秒更新时钟
./libexec/arcana-world-overlay-native -text '当前时间' -clock
# 每读到一行就替换全部文字，输入结束后保留最后一行
printf '%s\n' '新的提示' | ./libexec/arcana-world-overlay-native -stdin
# 循环显示示例弹幕
./libexec/arcana-world-overlay-native -demo
```

`-clock`、`-stdin`、`-demo` 只能选一个。`-stdin` 每行最多 1 MiB。

### 显示器

- `-list-displays`：以 JSON 列出显示器的 ID 和名称。
- `-displays '["ID1","ID2"]'`：在多个显示器上显示同样的内容。
- `-display` 或 `-output`：指定一个显示器。macOS 用 `-display <ID>`；Windows 用 `-display 2`（第二个显示器）或 `-output <设备名>`；Linux 用 `-output <输出名>`，例如 `DP-1`。

`-displays` 不能与 `-display`、`-output` 同时使用。

### 精美浮层参数

以下参数只能用于精美浮层：

| 参数 | 用途 |
| --- | --- |
| `-unlocked` | 允许拖动浮层 |
| `-reduced-motion` | 关闭动效 |
| `-idle-hide`、`-idle-seconds` | 无新消息时自动隐藏，以及等待秒数 |
| `-show-in-taskbar` | 在 macOS Dock 或 Windows 任务栏中显示 |
| `-align`、`-history-scale`、`-latest-scale`、`-line-spacing`、`-message-spacing`、`-history-alpha`、`-max-messages`、`-max-lines` | 排版 |
| `-gradient`、`-accent-start`、`-accent-end`、`-radius`、`-border-alpha`、`-shadow` | 渐变与面板 |
| `-animation`、`-animation-ms`、`-animation-distance` | 新消息动画 |

## 从源码构建

在项目根目录执行：

```sh
make overlay      # 原生浮层
make overlay-qt   # 精美浮层
make desktop      # 主程序、两种浮层和语音程序
```

`make desktop-native` 构建不含精美浮层的完整包。依赖安装和 Qt 准备见[从源码构建](../../docs/building.md#浮层)。
