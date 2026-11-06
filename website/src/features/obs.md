---
description: "将 Arcana World 连接到 OBS：开播时自动写入推流地址并开始推流；推流协议、手动配置及无画面排查。"
---

# OBS 与推流

连接 OBS 后可一键开播：Arcana World 先在 B 站开启直播间，再将推流地址写入 OBS 并开始推流，无需手动复制推流码。

| 终端界面 | 网页面板 |
| --- | --- |
| 按 `7` 打开「OBS」页 | 「直播台」右侧的「OBS Studio」卡片 |

需要 OBS Studio 28 或更高版本，场景、画面和声音照常在 OBS 中配置。

## 连接 OBS {#connect}

配置完成后，每次启动自动连接。

1. 在 OBS 中打开「工具」→「WebSocket 服务器设置」，勾选「开启 WebSocket 服务器」。

   ![OBS「工具」菜单中的「WebSocket 服务器设置」](/screenshots/obs-tools-menu.webp)

   ![OBS「WebSocket 服务器设置」：勾选「开启 WebSocket 服务器」，点击「显示连接信息」](/screenshots/obs-websocket-settings.webp)

2. 点击「显示连接信息」，点击「服务器密码」右侧的「复制」。

   ![OBS「WebSocket 连接信息」：复制服务器密码](/screenshots/obs-websocket-info.webp)

3. 回到 Arcana World，将密码填入「密码」，点击「保存」。
4. 点击顶栏的「连接 OBS」。顶栏显示「OBS 已连接」即连接成功。

![网页面板「OBS Studio」卡片：WebSocket 地址、密码、两个自动开关和推流协议](/screenshots/web-obs.webp)

地址通常无需修改，默认的 `ws://127.0.0.1:4455` 指向本机 OBS。仅当 OBS 中显示的端口不同，或 OBS 运行在另一台电脑上（如 `ws://192.168.1.20:4455`）时需要修改。

终端界面在「OBS」页选择「OBS 密码」填写，再选择「连接 OBS」，「连接状态」变为「已连接」即连接成功。

![终端界面「OBS」页，显示服务地址、连接状态和 OBS 操作](/screenshots/tui-obs.webp)

此处填写 OBS 的 WebSocket 密码，而非 B 站密码或推流码。无法连接时，确认 OBS 已运行、WebSocket 服务器已开启、密码为刚复制的密码。

## 开播流程 {#go-live}

「OBS Studio」卡片上的两个开关默认开启：

- 「自动连接 OBS」：启动时连接 OBS；开播时若尚未连接，先尝试连接一次。
- 「开播时自动推流」：推流地址写入 OBS 后，让 OBS 开始推流。

因此在默认设置下，点击「开始直播」即可直接播出。关闭「开播时自动推流」后，地址仍会写入 OBS，需在 OBS 中手动点击「开始直播」。

开播时若 OBS 已在推流，Arcana World 不会覆盖其推流目标，请先在 OBS 中停止推流再开播。提示「B 站已开播，但 OBS 联动失败」时，直播间已开启，按下文[没有画面](#no-video)排查。开播失败且提示中带有「B 站代码」时，见[常见问题](/reference/troubleshooting#bilibili-60045)。

## 没有画面 {#no-video}

直播间显示「直播中」仅表示 B 站已开启房间，OBS 是否在发送画面需单独检查：

1. 查看 OBS 右下角是否在推流。未推流时，在 OBS 中点击「开始直播」。
2. 检查 OBS 预览中是否有画面和声音。预览为黑屏时，先在 OBS 中修复场景和来源。
3. 预览正常但观众看不到：检查 OBS 是否报告推流错误，然后刷新 B 站直播间页面。

最近一次操作的结果显示在右下角通知和「设置」→「操作记录」中，终端界面见「日志」页（按 `9`）。

## 停播与退出 {#stop}

- 停播：点击顶栏「结束直播」（终端界面在「直播」页选择「停止 B 站直播」）。程序先停止 OBS 推流，再关闭直播间，OBS 本身保持运行。
- 断开 OBS：仅停止控制 OBS，正在进行的推流和直播间不受影响。
- 退出程序：默认停止 OBS 推流并关闭直播间。可在「设置」→「行为」中关闭「退出时停止 OBS 推流」和「退出时关闭直播间」。

关闭浏览器不会停播或退出程序。

## 推流协议 {#protocol}

默认使用 RTMP，通常无需修改。如需更换，在「设置」→「连接」→「推流协议」中选择，下次开播生效。

![网页面板「设置」→「连接」，含 OBS 连接、推流协议和网络代理](/screenshots/web-settings.webp)

::: details 其他协议说明
- 「SRT 优先，无 SRT 时退回 RTMP」：B 站提供 SRT 地址时使用 SRT，否则自动使用 RTMP。
- 「仅 SRT（不支持时报告错误）」：仅使用 SRT。B 站未提供 SRT 地址时直播间实际已开启，需先停播、更换协议后重新开播。
- RTMPSRT：OBS 推流到本机，由 Arcana World 转发给 B 站。OBS 必须与 Arcana World 在同一台电脑上，推流期间需保持程序运行、OBS 保持连接，断开或退出均会中断推流。还需先安装 libsrt 1.5 运行库：macOS 使用 `brew install srt`；Debian / Ubuntu 使用 `sudo apt install libsrt1.5-openssl`；其他 Linux 安装发行版的 libsrt 1.5 包；Windows 将与主程序架构一致的 `srt.dll` 及其依赖放入主程序同级的 `lib` 文件夹（可用 vcpkg 的 srt 包构建）。
:::

## 在 OBS 中手动填写推流码 {#manual}

也可以不让 Arcana World 控制 OBS（RTMPSRT 除外）：

1. 关闭「自动连接 OBS」和「开播时自动推流」，不连接 OBS。
2. 开播。
3. 在「直播台」点击「推流信息」，复制「服务器地址」和「推流码」。终端界面在「直播」页选择「显示 / 隐藏推流密钥」。
4. 在 OBS 的「设置」→「直播」中，服务选择「自定义…」，填写服务器和推流码，保存后点击「开始直播」。

::: warning 推流码要保密
获得推流码的人可以向你的直播间推流。请勿让推流码出现在截图、录屏或对外分享的日志中。
:::

::: details 命令行临时修改开关
启动时添加 `--obs-auto-connect=false` 或 `--obs-auto-stream=false`，仅在本次运行中关闭对应开关，已保存的设置不变。详见[命令行参数](/reference/cli)。
:::
