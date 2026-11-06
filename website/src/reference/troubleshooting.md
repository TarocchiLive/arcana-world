---
description: "按症状排查 Arcana World 的常见问题，以及查看日志和反馈问题的方法。"
---

# 常见问题

按症状查找对应条目。均无效时，参见[看日志、反馈问题](#logs)。

## 扫码登录失败 {#login}

须使用哔哩哔哩**手机 App** 扫码，并在手机上确认。二维码过期时，返回「账号」页重新扫码；终端中二维码显示不全时，按 `o` 以图片方式打开。

提示「暂时无法验证登录状态」时，通常是网络或代理问题，请检查后重试。

## 开播失败（B 站代码 60045） {#bilibili-60045}

![终端界面的警告：开播失败（B 站代码 60045）](/screenshots/error-60045.webp)

该错误由 B 站返回，表示账号或直播间受平台规则限制，官方直播姬同样会遇到，Arcana World 无法绕过。请按 B 站提示处理或联系 B 站客服。

其他带有「B 站代码」的错误提示同样由 B 站返回。

## OBS 无法连接 {#obs}

确认 OBS 已运行，并已在「工具」→「WebSocket 服务器设置」中勾选启用。点击「显示连接信息」，将密码复制到 Arcana World 后重新连接。此处需要的是 OBS 的服务器密码，而非 B 站密码或推流密钥。

更多说明见 [OBS 与推流](/features/obs)。

## 已开播但观众看不到画面 {#no-video}

B 站直播间开启不代表 OBS 正在推流。请在 OBS 中确认推流状态；未开启「开播时自动启动 OBS 推流」时，需要在 OBS 中手动开始推流。

OBS 正在推流但画面黑屏或无声音时，检查 OBS 中的场景、来源和混音器。

## 收不到弹幕 {#no-chat}

确认当前账号和直播间正确，且弹幕监听已开启。终端界面出现连接错误时，在「弹幕」页按 `r` 重试。

某条消息始终不出现时，检查是否被[屏蔽词](/features/blocklist)过滤。程序未运行期间发送的弹幕不会补录。

## 浮层不显示 {#overlay}

浮层显示在运行 Arcana World 的电脑上。请确认浮层已开启且选择了正确的显示器。开启「无新消息时自动隐藏」后，无新消息时浮层自动隐藏，收到新消息时重新显示。

仍无法显示时，运行 `arcana-world --doctor --overlay` 检查辅助程序是否存在。系统支持情况见[弹幕浮层](/features/overlay)。

## 没有声音 {#no-sound}

先点击试听。试听无声音时，检查本机的输出设备和音量；试听正常但直播时不播报时，确认语音播报已开启，且已勾选需要播报的事件。

在 macOS 上点击试听完全无反应时，通常是语音辅助程序被系统拦截，见下文 [macOS 提示无法打开](#macos-blocked)。

## 网页面板无法打开 {#webui}

终端中会显示面板地址（默认 `http://127.0.0.1:5273`），浏览器未自动打开时，手动复制该地址访问。提示端口被占用时，改用其他端口启动：`arcana-world --gui --web-port 6000`。

关闭浏览器不会停止程序，重新访问同一地址即可恢复。页面显示「连接中断」时，检查启动程序的终端是否仍在运行。

## Windows 10 终端界面乱码 {#win10-garbled}

窗口顶部和底部出现多行重复内容，原因是 Windows 10 自带的旧版控制台无法正确显示终端界面。安装 Windows Terminal 后重新打开即可，具体步骤见[终端入门](/guide/terminal)。

## macOS 提示无法打开 {#macos-blocked}

macOS 会为从网络下载的程序添加隔离标记。在解压后的文件夹中打开终端，运行：

```sh
xattr -dr com.apple.quarantine .
./arcana-world
```

该命令同时放行浮层和语音辅助程序。每次升级到新文件夹后，都需要重新运行。

## 提示数据目录被占用 {#directory-occupied}

表示已有一个 Arcana World 正在运行，例如浏览器已关闭但终端仍在运行的实例。退出该实例后重新启动。

## 设置无法修改 {#locked-settings}

启动时添加了 `--tts`、`--proxy` 等参数时，对应设置在本次运行中为只读，不带参数重启后即可修改，见[命令行参数](/reference/cli#overrides)。正在连接 OBS 或开播时，相关设置也会暂时锁定，操作完成后解除。

## 看日志、反馈问题 {#logs}

最近的操作结果位于终端界面按 `9` 打开的「日志」页，或网页面板的「设置」→「操作记录」中。更详细的原因记录在日志文件中：

| 系统 | 日志位置 |
| --- | --- |
| macOS / Linux | `~/.arcana/world/logs/arcana-world.log` |
| Windows | `C:\Users\用户名\.arcana\world\logs\arcana-world.log` |

如需记录更多细节，使用 `arcana-world --log-level debug` 启动（网页面板需再加 `--gui`），然后重现问题。

### 提交 Issue {#report}

通过 [GitHub Issues](https://github.com/TarocchiLive/arcana-world/issues) 或 QQ 群（见[参与开发](/development/#feedback)）反馈时，请附上以下信息，以便定位问题：

- `arcana-world --version` 的输出、系统版本，以及使用的是终端界面还是网页面板。
- 操作步骤、预期结果、实际结果，以及错误提示原文。
- `arcana-world --doctor` 的输出和问题发生时段的日志。

::: warning 发送前检查
日志会自动隐去密码和推流密钥，但可能仍包含用户名、直播间和弹幕内容，发送前请检查。请勿发送扫码登录链接、推流密钥、API Key 和 `credentials/secrets.json`。
:::
