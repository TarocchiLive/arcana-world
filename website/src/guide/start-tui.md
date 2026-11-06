---
description: "使用终端界面完成第一次开播：界面布局、扫码登录、选择分区、连接 OBS、开播和停播。"
---

# 快速上手：终端界面

本篇介绍如何通过键盘操作完成第一次开播。Windows 10 用户请先阅读[终端入门](/guide/terminal)并安装 Windows Terminal，否则界面可能显示异常。

开始前请准备：手机上的哔哩哔哩 App（账号已开通直播间），以及已安装的 OBS Studio（28 或更新版本），场景中已配置画面和声音。

## 1. 启动 {#launch}

Windows 在解压后的文件夹中右键选择「在终端中打开」，输入 `.\arcana-world.exe` 并回车；macOS 和 Linux 在终端中运行：

```sh
./arcana-world
```

程序打开「直播」页，状态栏显示「就绪 · 登录后可管理直播间」。

![终端界面「直播」页，尚未登录](/screenshots/tui-live.webp)

## 2. 界面布局 {#layout}

左侧为页面列表，每页前有编号，按对应数字键即可跳转，例如 `5` 为「账号」、`7` 为「OBS」。用 `↑` `↓` 选择操作，`Enter` 打开，`Esc` 返回。

底部一行显示当前可用的按键。按 `0` 打开帮助，其中包含「新手入门：完成第一次开播」。

![按 0 打开的帮助页](/screenshots/tui-help.webp)

支持鼠标操作：点击页面名称或操作，滚轮滚动内容。更多按键见[终端入门](/guide/terminal#keys)。

## 3. 扫码登录 {#sign-in}

按 `5` 打开「账号」页，选择「扫码添加账号」，屏幕上显示二维码。

![「账号」页的登录二维码](/screenshots/tui-login.webp)

使用哔哩哔哩手机 App 扫码，并在手机上确认登录。二维码显示不完整时，按 `o` 以图片方式打开，或在手机上打开二维码下方的链接。

## 4. 设置标题和分区 {#room}

按 `6` 打开「直播间」页：

- 选择「标题（编辑 / 最近使用）」，输入标题后按 `Enter`。
- 选择「分区（最近 / 分类 / 搜索）」，选定一个分区。**必须选择分区**，否则无法开播。

![「直播间」页的标题和分区](/screenshots/tui-room.webp)

## 5. 连接 OBS {#obs}

在 OBS 中打开「工具」→「WebSocket 服务器设置」，勾选启用，然后点击「显示连接信息」复制密码。

![OBS「工具」菜单中的「WebSocket 服务器设置」](/screenshots/obs-tools-menu.webp)

![OBS「WebSocket 服务器设置」：勾选「开启 WebSocket 服务器」，点击「显示连接信息」](/screenshots/obs-websocket-settings.webp)

![OBS「WebSocket 连接信息」：复制服务器密码](/screenshots/obs-websocket-info.webp)

返回 Arcana World，按 `7` 打开「OBS」页，选择「OBS 密码」并粘贴密码，再选择「连接 OBS」。地址通常无需修改。

![「OBS」页](/screenshots/tui-obs.webp)

页面显示「已连接」即表示连接成功。无法连接时，请检查密码是否正确，以及 OBS 中是否已勾选启用；具体原因可按 `9` 在「日志」中查看。

## 6. 开播 {#go-live}

按 `1` 返回「直播」页，核对标题和分区。如显示「暂无直播间信息」，先选择「刷新直播间状态」。

![登录后的「直播」页，「开播」位于操作列表第一项](/images/arcana-world-01.webp)

选择「开播」，在确认框中核对内容，选中「确认执行」后按 `Enter`。

Arcana World 会打开直播间，将推流地址写入 OBS，并让 OBS 开始推流。显示「B 站已开播，OBS 已同步推流」即表示开播成功。建议用手机打开自己的直播间，确认画面和声音正常。

B 站可能要求先验证身份，按提示用手机扫码验证后，再次开播。开播失败且提示中包含「B 站代码」时，参见[常见问题](/reference/troubleshooting#bilibili-60045)。

## 7. 停播和退出 {#stop}

直播结束后在「直播」页选择「停止 B 站直播」，确认后直播间关闭，OBS 停止推流（OBS 本身不会关闭）。

按 `q` 并在确认框中选择「退出应用」即可退出程序。退出时默认停止 OBS 推流并关闭直播间，可按 `8` 打开「设置」，关闭「退出时停止 OBS 推流」和「退出时关闭直播间」。

![「设置」页里的退出开关](/screenshots/tui-settings.webp)

## 后续步骤 {#next}

- [弹幕浮层](/features/overlay)：在桌面上显示弹幕。
- [语音播报](/features/tts)：语音播报弹幕和礼物。
- [弹幕与观众管理](/features/chat)：在「弹幕」页回复弹幕、管理房管和禁言。
