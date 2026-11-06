---
description: "Arcana World 的定位、功能，以及终端界面与网页面板的选择。"
---

# 了解 Arcana World

Arcana World 是哔哩哔哩直播辅助工具。通过手机扫码登录，连接 OBS 后可一键开播；直播期间接收弹幕、在桌面显示弹幕浮层，并语音播报弹幕和礼物。

![终端界面「直播」页](/images/arcana-world-01.webp)

## 功能 {#features}

- **开播和停播**：修改标题、选择分区、更换封面；开播时自动将推流地址写入 OBS 并开始推流，停播时同步停止。详见[直播间设置](/features/room)、[OBS 与推流](/features/obs)。
- **弹幕与观众管理**：查看弹幕、礼物和醒目留言，在电脑上直接回复，管理房管、禁言和[屏蔽词](/features/blocklist)。详见[弹幕与观众管理](/features/chat)。
- **桌面弹幕浮层**：半透明显示在屏幕上，鼠标可穿透，不影响游戏操作。详见[弹幕浮层](/features/overlay)。
- **语音播报**：朗读弹幕和礼物，支持云端语音，也可下载模型在本地合成。详见[语音播报](/features/tts)。

画面和声音的采集与推流仍由 OBS Studio 负责，Arcana World 负责直播间和弹幕管理。Arcana World 是独立开发的开源软件（GPL-3.0-only），并非 B 站官方产品。

![桌面弹幕浮层的效果](/images/arcana-world-05.webp)

## 界面选择 {#interfaces}

程序提供两种界面，功能基本一致，设置共用：

- **网页面板**：在浏览器中通过鼠标操作，上手门槛最低。推荐不熟悉命令行的用户使用。
- **终端界面**：在终端窗口中通过键盘操作，占用资源少、启动快，可缩小放在 OBS 旁边。其特点见[终端入门](/guide/terminal)。

Arcana World 支持 Windows（x64、ARM64）、Apple 芯片的 Mac，以及 Linux（x64、ARM64）。

## 后续步骤 {#next}

1. [安装](/guide/install)程序。
2. 选择一篇快速上手，完成第一次开播：
   - [快速上手：网页面板](/guide/start-web)
   - [快速上手：终端界面](/guide/start-tui)

遇到问题请参见[常见问题](/reference/troubleshooting)。
