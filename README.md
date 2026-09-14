
# Arcana World

<p align="center">
  <img src="assets/arcana-world.webp" alt="Arcana World 世界塔罗牌图标" width="180">
</p>


[![CI](https://github.com/TarocchiLive/arcana-world/actions/workflows/ci.yml/badge.svg)](https://github.com/TarocchiLive/arcana-world/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-GPL--3.0--only-blue)](LICENSE)
[![Release](https://img.shields.io/github/v/release/TarocchiLive/arcana-world?sort=semver&cacheSeconds=300)](https://github.com/TarocchiLive/arcana-world/releases/latest)

**中文** · [English](README.en.md)

Arcana World 是基于 Go 的 bilibili 直播姬 TUI 替代。方便用于无法直接运行直播姬的平台（如Linux），或在使用官方直播姬遇到障碍时开播，用户可在终端轻松完成登录和直播设置，直接使用 OBS 推流开播。

## 预览

![主菜单](assets/arcana-world-1-cn.png)
![封面图预览](assets/arcana-world-2-cn.png)

## 功能详情

- 扫码登录，保存和切换多个账号。
- 开播、停播，修改标题、分区、公告和封面。
- 自动裁剪封面，支持终端图片预览（需要终端模拟器支持，建议使用 kitty 或 ghostty ）后再上传。
- 将推流配置写入 OBS，按需同步开停播。
- 登录后自动监听自己的直播间，显示弹幕、礼物、醒目留言（SC）和大航海事件。
- 支持 RTMP / SRT、中英文界面和网络代理。
- 跨平台支持：macOS、Windows 和 Linux 均可使用。

## 安装

从 [最新版本](https://github.com/TarocchiLive/arcana-world/releases/latest) 下载，解压后运行：

| 平台 | x64 | ARM64 |
| --- | --- | --- |
| macOS | [Intel](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-darwin-amd64.tar.gz) | [Apple Silicon](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-darwin-arm64.tar.gz) |
| Windows | [下载](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-windows-amd64.zip) | [下载](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-windows-arm64.zip) |
| Linux | [下载](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-linux-amd64.tar.gz) | [下载](https://github.com/TarocchiLive/arcana-world/releases/latest/download/arcana-world-linux-arm64.tar.gz) |

也可以使用 Go 1.26+ 从源码构建：

```sh
git clone https://github.com/TarocchiLive/arcana-world.git
cd arcana-world
make build
./bin/arcana-world
```

账号凭据使用系统密钥环保存。Linux 用户需要可用且已解锁的 Secret Service。

## 开始直播

1. 在「账号」页用哔哩哔哩 App 扫码登录。
2. 在「直播间」页设置标题、分区和封面。
3. 在 OBS Studio 顶栏点击「工具」→「WebSocket 服务器设置」→ 勾选「开启 WebSocket 服务器」→「显示连接信息」→ 复制服务器密码 →「确定」（[官方设置指南](https://obsproject.com/kb/remote-control-guide)）。
   OBS 28 及以上已内置此功能，无需安装插件；旧版请安装与 OBS 版本兼容的 [obs-websocket 5.x 插件](https://github.com/obsproject/obs-websocket/releases)。
4. 在 Arcana World 的「OBS」（`5`）页填写服务器地址（例如 `ws://127.0.0.1:4455`）和刚才复制的密码。
5. 在同一页连接，开启「自动推流」；代理和推流协议在「设置」（`6`）页调整。
6. 回到「直播」页，选择开播。

结束时选择「停播」。如果没有开启自动推流，需要手动在 OBS 中开始和停止推流。

如果 B 站要求身份验证，按提示完成后重新开播。

### 快捷键

| 按键 | 操作 |
| --- | --- |
| `↑↓` / `j k` | 选择 |
| `Enter` | 确认 |
| `Tab` / `1–8` | 切换页面 |
| `PgUp` / `PgDn` | 滚动 |
| `r` | 刷新 |
| `Esc` | 返回或取消 |
| `q` | 退出 |

页面顺序：`1` 直播、`2` 弹幕、`3` 账号、`4` 直播间、`5` OBS、`6` 设置、`7` 日志、`8` 帮助。

### 弹幕与历史

登录后自动监听自己的直播间，按 `2` 浏览弹幕与历史，切换页面不影响监听。

- `s` 开关监听；`Space` 暂停显示 / 跟随最新。
- `[` / `]` 翻页；`End` 回到最新；`o` 切换历史房间。
- `f` 开关白名单内的附加事件；`r` 刷新 / 重试；`PgUp` / `PgDn` 滚动。

弹幕菜单默认展示的消息类型详见[弹幕显示白名单](docs/danmaku-whitelist.md)。白名单之外的消息即使开启 `f` 也不展示，但原始数据仍保存。

历史按房间保存在数据目录的 `danmaku/history.db`，与操作日志均保留最近七天并自动清理。

断开连接时产生的消息不会被重新拉取同步到数据库。

### 常用选项

界面默认跟随系统语言，也可以手动选择：

```sh
arcana-world --lang zh-CN               # 使用中文
arcana-world --lang en                  # 使用英文
arcana-world --config-dir /path/to/data # 指定数据目录
arcana-world --help                     # 查看帮助
```

默认数据目录：`~/.arcana/world`。

## TODO
- [ ] 支持获取b站推流直链，自动连接 OBS，自动开播。
- [x] 支持b站直播间弹幕抓取，提供 TUI 内礼物，聊天记录浏览。
- [ ] 支持b站直播间弹幕浮层。
- [ ] 支持直播姬，TTS 朗读弹幕，礼物播报。

## 开发

```sh
make run    # 本地运行
make check  # 测试与静态检查
make build  # 构建
```

翻译词表位于 [`internal/i18n/locales`](internal/i18n/locales)，按语言和模块划分。欢迎通过 [Issue](https://github.com/TarocchiLive/arcana-world/issues) 或 Pull Request 反馈问题、参与改进。

## 声明

本项目是独立开发的开源工具，使用者应自行遵守相关法律法规及相关平台规则。本项目仅用作 golang 与相关框架学习与交流，请勿用作其他用途。


## 许可证

采用 [GPL-3.0-only](LICENSE)。

## 致谢

- 社区内无私提供声明与实现参考的各位大佬。
- [Radekyspec/StartLive](https://github.com/Radekyspec/StartLive)：本项目推流协议实现的来源。
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)：终端界面框架。
