---
description: "在电脑屏幕上显示半透明的实时弹幕：开启浮层、选择显示器和事件、调整外观，以及浮层不可见时的排查方法。"
---

# 弹幕浮层

浮层在屏幕的固定位置显示实时弹幕。浮层半透明、不抢占焦点，鼠标点击可穿透到下层窗口，不影响游戏操作。

![桌面弹幕浮层的实际效果](/images/arcana-world-05.webp)

| 界面 | 入口 |
| --- | --- |
| 终端界面 | 按 `3` 打开「弹幕浮层」页 |
| 网页面板 | 打开「输出」 |

浮层显示在运行 Arcana World 的电脑上，随主程序退出而关闭。下文使用终端界面中的名称，网页面板「输出」页提供相同的设置。

## 浮层类型

- **原生浮层**（默认）：纯文字样式，可调整字体、颜色、描边和透明度。
- **精美浮层**：增加消息入场动画、渐变、圆角和阴影，并支持无新消息时自动隐藏。

两种浮层显示的内容相同，外观设置分别保存。多数平台两种浮层均支持，例外见下表：

| 平台 | 原生浮层 | 精美浮层 |
| --- | --- | --- |
| macOS（Apple 芯片） | 支持 | 支持 |
| Windows x64（10 1803 及以上） | 支持 | 支持 |
| Windows ARM64 | 支持 | 不支持 |
| Linux Wayland（KDE Plasma、niri 等） | 支持 | 支持 |
| GNOME Wayland | 不支持 | 安装扩展后支持，见[下文](#gnome) |
| Linux X11 | 不支持 | 不支持 |

## 开启浮层 {#enable}

1. 在「浮层样式」中选择「原生浮层」或「精美浮层」，然后打开「浮层」开关（网页面板中为「桌面浮层」）。
2. 在「选择浮层显示器」中勾选要显示浮层的屏幕。支持多选，各屏幕显示相同内容。
3. 在「选择事件」中勾选要在浮层上显示的内容，如弹幕、礼物、醒目留言。

在勾选列表中用 `↑` `↓` 移动，按 `Enter` 或 `Space` 勾选，修改即时保存。

![终端界面「弹幕浮层」页](/screenshots/tui-overlay.webp)

页面显示「浮层：运行中」时，屏幕上即出现浮层。未开播或暂无弹幕时，可打开「示例预览」确认浮层位置。

浮层与语音播报的事件分别选择，例如浮层显示所有弹幕，语音只播报礼物。

## 外观设置 {#appearance}

位置、尺寸、字体、颜色和透明度均在「浮层内容与外观」中设置。浮层运行时，修改会实时反映在浮层上：按 `Enter` 保存，按 `Esc` 撤销；数值框中可用 `↑` `↓` 微调。

![网页面板「输出」页的「位置与尺寸」](/screenshots/web-outputs-overlay.webp)

常用设置：

- **示例预览**：在浮层上显示普通弹幕、舰长、醒目留言、礼物等示例，无需登录，且不会被播报。建议调整外观时开启。
- **屏幕锚点**：选择浮层贴靠的屏幕角落或边缘，再通过水平、垂直偏移微调。
- **内容模式**：默认显示实时弹幕，可切换为「房间状态」或「状态与弹幕」。
- **无新消息时自动隐藏**（精美浮层）：一段时间内无新消息时浮层淡出，收到新消息后淡入。

点击「恢复默认外观」可重置外观设置，开关和事件选择保持不变。

如需修改每条消息显示的文字，参见[自定义浮层与播报文字](/features/templates)。

## 在 OBS 中捕获浮层 {#obs-capture}

如需将浮层作为独立来源加入直播画面，请使用精美浮层：

1. 在「浮层内容与外观」中打开「在 Dock 中显示」（macOS）或「在任务栏中显示」（Windows）。
2. 在 OBS 中添加窗口捕获来源，选择标题为 `Arcana World · 精美浮层 / Elegant overlay` 的窗口。

该选项在 Linux 上不生效。

## 浮层不可见 {#not-visible}

首先查看「弹幕浮层」页上的状态和「浮层最近错误」。状态为「失败」时，关闭「浮层」开关后重新打开。状态正常但浮层不可见时，依次检查：

1. 「选择浮层显示器」中至少勾选了一块已连接的屏幕。新接入的显示器需在列表中按 `r` 刷新。
2. 弹幕监听已开启，且「选择事件」中勾选了对应事件。参见[弹幕与观众管理](/features/chat)。
3. 消息未被[屏蔽词](/features/blocklist)过滤。
4. 文字不透明度不为 0，且位置未移出屏幕。可打开「示例预览」确认。
5. 精美浮层开启自动隐藏时，无新消息期间浮层处于隐藏状态。

浮层不保证能显示在锁屏界面或独占全屏的游戏之上。

## 在 GNOME 上使用精美浮层 {#gnome}

在 GNOME Wayland 上，精美浮层需要配套的 GNOME Shell 扩展才能固定位置并保持置顶。支持 GNOME 45 至 50。

1. 在 Linux 发行包的解压目录中执行：

   ```sh
   dest="${XDG_DATA_HOME:-$HOME/.local/share}/gnome-shell/extensions/arcana-overlay@arcana-world"
   mkdir -p "$dest"
   cp extensions/gnome/arcana-overlay@arcana-world/{metadata.json,extension.js} "$dest/"
   ```

2. 注销并重新登录。
3. 启用扩展：

   ```sh
   gnome-extensions enable arcana-overlay@arcana-world
   ```

4. 返回「弹幕浮层」页，选择「精美浮层」并打开开关。

升级 Arcana World 后，重新执行第 1 步并重新登录。锁屏会关闭浮层，解锁后需关闭并重新打开开关。

::: details GNOME 上的错误信息
| 错误信息包含 | 处理 |
| --- | --- |
| `enable the Arcana Elegant GNOME Shell extension` | 扩展未运行。确认已安装并重新登录后，再执行启用命令 |
| `An overlay owner is already active` | 已有一个精美浮层在运行，请先将其关闭 |
| `extension disabled or service owner changed`、`extension disconnected` | 已锁屏或扩展已被禁用。恢复后关闭并重新打开开关 |
| `initial placement timed out`、`Window placement timed out` | 浮层窗口未能及时显示，关闭并重新打开开关以重试 |

卸载扩展：`gnome-extensions uninstall arcana-overlay@arcana-world`。
:::

::: details 单独运行浮层程序
浮层程序可脱离主程序单独运行，用于显示提示文字或时钟。程序位于发行包的 `libexec` 文件夹中，原生浮层为 `arcana-world-overlay-native`（Windows 上带 `.exe` 后缀）。

```sh
# 在左下角显示一段欢迎语
./libexec/arcana-world-overlay-native -text '欢迎来到直播间' -anchor bottom-left -x 24 -y 24
# 显示时钟
./libexec/arcana-world-overlay-native -text '当前时间' -clock
```

按 `Ctrl+C` 关闭，使用 `-help` 查看全部参数。
:::
