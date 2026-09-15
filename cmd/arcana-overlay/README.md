# Arcana Overlay

**中文** · [English](README.en.md)

Arcana World 的跨平台桌面文字浮层。可在屏幕固定区域显示文字，无焦点，支持鼠标穿透，不影响其他交互。

可以独立显示提示、时钟或持续更新的文字，也可以随 Arcana World 启动，显示直播间标题和开播状态。**目前尚未接入 B 站弹幕。**

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

默认显示在屏幕左侧、垂直居中，距左侧 32，大小为 420 × 180。

```sh
./bin/arcana-overlay -text '欢迎来到直播间' -anchor bottom-left -x 24 -y 24 -width 480 -height 200 -padding 20 -font-size 28 -background-alpha 0.25
```

| 参数 | 用途 |
| --- | --- |
| `-text '文字'` | 要显示的内容 |
| `-anchor left` | 选择屏幕上的位置，见下方九宫格 |
| `-x 32 -y 0` | 横向、纵向偏移；靠边时正数向屏幕内侧移动，居中时正数向右、向下移动 |
| `-width 420 -height 180` | 浮层宽高，必须大于 0 |
| `-padding 12,16,12,16` | 按上、右、下、左设置留白；一个数表示四边相同 |
| `-font '字体名称'` | 选择字体，不设置则使用系统默认字体 |
| `-font-size 22 -font-weight 500` | 字号和字重；字重范围为 100–900 |
| `-italic` | 使用斜体 |
| `-text-alpha 0.9` | 文字不透明度，0 为完全透明，1 为不透明 |
| `-background-alpha 0.3` | 背景不透明度；设为 0 则只显示文字 |

位置名称：

| 左 | 中 | 右 |
| --- | --- | --- |
| `top-left` | `top` | `top-right` |
| `left` | `center` | `right` |
| `bottom-left` | `bottom` | `bottom-right` |

位置、大小和留白使用随屏幕缩放的逻辑单位。文字在浮层范围内换行，超出高度的内容可能被裁掉；可以增加高度或减小字号。

### 查看动态更新

显示每秒更新的时钟：

```sh
./bin/arcana-overlay -text '当前时间' -clock
```

通过管道接收其他程序输出的文字：

```sh
printf '%s\n' '新的提示内容' | ./bin/arcana-overlay -stdin
```

`-stdin` 每收到一行就替换全部文字，输入结束后保留最后一行，不会自动关闭。单行最多约 1 MiB。它不能与 `-clock` 同时使用。

### 选择显示器

默认使用程序启动时选中的显示器，也可手动指定：

- macOS：`-display <显示器 ID>`。
- Windows：`-display 2` 选择第二个显示器，或用 `-output <设备名>`。
- Linux：`-output DP-1` 等桌面中的实际输出名称。

`-display` 和 `-output` 不要同时使用。

### 随 Arcana World 启动

将浮层程序放在主程序旁边，然后启用：

```sh
./bin/arcana-world -overlay
```

此时显示直播间标题和开播状态，并随主程序退出而关闭。浮层启动失败不影响主程序的其他功能，可在日志页查看原因。

如果两个程序不在同一目录，用 `-overlay-executable` 指定浮层路径。此模式下位置和外观由主程序管理；上面的外观参数用于独立运行。

## 构建与启动

**以下为开发者帮助，arcana-overlay默认已集成于arcana-world并提供原生二进制，无需额外依赖。**

浮层需单独构建，当前不包含在主程序的发布包中。以下命令均在**项目根目录**执行，需先安装项目要求版本的 Go。

macOS 还需安装 Xcode Command Line Tools；Linux 需安装 Wayland、Pango/Cairo 开发库和 `pkg-config`。Debian/Ubuntu 可使用：

```sh
sudo apt-get install libwayland-dev libpango1.0-dev libcairo2-dev pkg-config
```

macOS / Linux：

```sh
make overlay
./bin/arcana-overlay -text '欢迎来到直播间'
```

Windows（PowerShell）：

```powershell
go build -o bin/arcana-overlay.exe ./cmd/arcana-overlay
.\bin\arcana-overlay.exe -text '欢迎来到直播间'
```

在启动它的终端按 `Ctrl+C` 关闭。也可以加上 `-duration 30s`，在 30 秒后自动关闭。下方示例使用 macOS / Linux 的命令写法，Windows 请替换为 `.\bin\arcana-overlay.exe`。

## 更多选项

```sh
./bin/arcana-overlay -help
```

帮助中的 `-socket` 由主程序管理浮层时使用，普通运行无需设置。
