---
description: "使用网页面板完成第一次开播：扫码登录、选择分区、连接 OBS、开播和停播。"
---

# 快速上手：网页面板

本篇介绍如何通过鼠标操作完成第一次开播。开始前请准备：

- 手机上的哔哩哔哩 App，账号已开通直播间。
- 电脑上的 OBS Studio（28 或更新版本），场景中已配置画面和声音。

另有[视频教程](https://www.bilibili.com/video/BV1VbeN6tEtn)可供参考。

## 1. 打开网页面板 {#open}

Windows 上双击解压文件夹中的 `arcana-world-webui.exe`；macOS 和 Linux 在终端中运行：

```sh
./arcana-world --gui
```

浏览器会自动打开面板；如未打开，请手动访问 `http://127.0.0.1:5273`。启动时弹出的命令行窗口即程序本身，请保持其运行。

![网页面板「直播台」，尚未登录](/screenshots/web-studio.webp)

右上角的主按钮显示当前待完成的步骤，初始为「登录账号」。

::: tip 默认打开网页面板
运行 `arcana-world --default-gui` 后，直接启动 `arcana-world` 即打开网页面板。
:::

## 2. 扫码登录 {#sign-in}

点击左侧的「账号」，再点击「扫码登录」。

![「账号」页的「扫码登录」按钮](/screenshots/web-accounts.webp)

屏幕上显示二维码后，使用哔哩哔哩手机 App 扫码并在手机上确认登录，账号即出现在列表中。

![扫码登录弹窗](/screenshots/web-login.webp)

手机无法扫码时，点击「复制验证链接」，发送到手机并用浏览器打开。

## 3. 确认直播间并选择分区 {#room}

返回「直播台」，「直播间」卡片显示房间号、标题和分区。如显示尚未载入，点击「刷新房间」。

![登录后的「直播台」](/images/arcana-world-06.webp)

在「标题」框中修改标题，按 `Enter` 保存。**必须选择分区**，否则无法开播：点击「分区」，搜索名称并选择具体分区。

## 4. 连接 OBS {#obs}

在 OBS 中打开「工具」→「WebSocket 服务器设置」，勾选启用，然后点击「显示连接信息」复制密码。

![OBS「工具」菜单中的「WebSocket 服务器设置」](/screenshots/obs-tools-menu.webp)

![OBS「WebSocket 服务器设置」：勾选「开启 WebSocket 服务器」，点击「显示连接信息」](/screenshots/obs-websocket-settings.webp)

![OBS「WebSocket 连接信息」：复制服务器密码](/screenshots/obs-websocket-info.webp)

返回面板，在「直播台」的「OBS Studio」卡片中将密码粘贴到「密码」框，点击「保存」，再点击顶栏的「连接 OBS」。地址通常无需修改。

![「OBS Studio」卡片](/screenshots/web-obs.webp)

连接成功后，顶栏显示「OBS 已连接」。右侧「开播准备」中各项均完成后即可开播。

![开播准备全部完成的「直播台」](/screenshots/web-studio-ready.webp)

无法连接时，请检查密码是否正确，以及 OBS 中是否已勾选启用。此处需填写 OBS 的服务器密码，而非 B 站密码。

## 5. 开始直播 {#go-live}

点击顶栏右上角的「开始直播」，在弹出的确认框中核对内容，再点击「开播」。

![开播确认框](/screenshots/web-golive-confirm.webp)

Arcana World 会打开直播间，将推流地址写入 OBS，并让 OBS 开始推流。显示「B 站已开播，OBS 已同步推流」即表示开播成功。建议用手机打开自己的直播间，确认画面和声音正常。

B 站可能要求先验证身份，按提示用手机扫码验证后，再次点击「开始直播」。开播失败且提示中包含「B 站代码」时，参见[常见问题](/reference/troubleshooting#bilibili-60045)。

## 6. 停播和退出 {#stop}

直播结束后点击顶栏的「结束直播」，确认后直播间关闭，OBS 停止推流（OBS 本身不会关闭）。

![直播中的「直播台」，顶栏显示「结束直播」](/screenshots/web-live.webp)

关闭浏览器不会停播或退出程序，重新访问面板地址即可恢复。在启动程序的命令行窗口按 `Ctrl+C` 退出。退出时默认停止 OBS 推流并关闭直播间，可在「设置」→「行为」中关闭这两项。

## 后续步骤 {#next}

- [弹幕浮层](/features/overlay)：在桌面上显示弹幕。
- [语音播报](/features/tts)：语音播报弹幕和礼物。
- [弹幕与观众管理](/features/chat)：回复弹幕、管理房管和禁言。
