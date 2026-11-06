---
description: "终端的概念、终端界面的特点、Windows 10 乱码的解决方法，以及终端界面的基本按键。"
---

# 终端入门

## 什么是终端 {#what}

终端是通过文字和键盘操作程序的窗口，各系统均自带：Windows 上称为「终端」或「命令提示符」，macOS 上称为「终端」，Linux 上通常也称为「终端」。在其中输入命令并按回车即可运行程序。

## 终端界面的特点 {#why-tui}

与网页面板相比，终端界面具有以下特点：

- **轻量**：常驻内存约 40 MB，无需打开浏览器。
- **快速**：启动后即可使用。
- **全键盘**：数字键切换页面、方向键选择，无需离开键盘。
- **小巧**：窗口可缩至很小，便于与 OBS 并排放置。

两种界面功能基本一致、设置共用，可按使用习惯选择。

## 终端选择 {#which}

- **Windows**：使用 Windows Terminal。Windows 11（22H2 起）默认使用它；Windows 10 需自行安装，见下一节。
- **macOS**：可使用自带的「终端」。使用 iTerm2、Ghostty 或 kitty 时，可在终端中直接显示封面图。
- **Linux**：可使用发行版自带的终端。kitty 和 Ghostty 同样可直接显示封面图。

在其他终端中预览封面时，会以字符绘制粗略图像；Windows 上的终端界面不支持封面预览，如需查看封面请使用网页面板。

## Windows 10 显示乱码 {#win10}

在 Windows 10 上双击 `arcana-world.exe` 后，窗口顶部和底部可能出现多行重复内容：

![Windows 10 旧版控制台中显示错乱的终端界面](/screenshots/win10-conhost-garbled.webp)

原因是 Windows 10 默认使用旧版控制台窗口运行命令行程序。在中文系统下，旧版控制台会将 ←→↑↓ 箭头、部分框线等“宽度不确定”的字符按两格宽度绘制，而程序按一格排版。最下面一行因此超出窗口宽度，控制台自动换行并将整屏上推；每次刷新重绘后，就叠加出多行重复内容。Windows Terminal 按一格宽度显示这些字符，不会出现此问题。

### 安装 Windows Terminal {#install-wt}

在 Microsoft Store 中搜索「Windows Terminal」安装，或在命令行中运行：

```powershell
winget install --id Microsoft.WindowsTerminal -e
```

### 在 Windows Terminal 中运行 {#run-in-wt}

1. 打开解压后的 Arcana World 文件夹，在空白处右键，选择「在终端中打开」（Windows 10 上为「在 Windows 终端中打开」）。
2. 输入以下命令并按回车：

   ```powershell
   .\arcana-world.exe
   ```

如需双击时也使用 Windows Terminal 打开：在 Windows Terminal 的「设置」→「启动」中，将「默认终端应用程序」设为 Windows Terminal。该选项仅在较新的 Windows 10 和 Windows 11 中提供，没有该选项时请使用右键方式启动。

也可以双击 `arcana-world-webui.exe` 直接使用网页面板，见[快速上手：网页面板](/guide/start-web)。

## 基本操作 {#keys}

| 按键 | 作用 |
| --- | --- |
| `1`–`9` | 跳到对应页面 |
| `Tab`、`←` `→` | 切换页面 |
| `↑` `↓` | 选择操作 |
| `Enter` | 打开选中的操作 |
| `Esc` | 返回或取消 |
| `z` | 隐藏页面列表和底栏，再按一次恢复 |
| `0` | 帮助 |
| `q` | 退出（需确认） |

支持鼠标操作：点击页面名称或操作即可打开，滚轮滚动内容。复制弹幕或日志中的文字时，用鼠标拖选后按 `y`。

在输入框、弹窗和「弹幕」页中按键有所不同，以底部一行的提示为准。
