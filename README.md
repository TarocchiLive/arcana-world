
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

## 功能详情

- 扫码登录，保存和切换多个账号。
- 开播、停播，修改标题、分区、公告和封面。
- 自动裁剪封面，预览后再上传。
- 将推流配置写入 OBS，按需同步开停播。
- 登录后自动监听自己的直播间，显示弹幕、礼物、醒目留言（SC）和大航海事件；支持开关及本地历史浏览。
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
4. 在 Arcana World 的「设置」页填写服务器地址（例如 `ws://127.0.0.1:4455`）和刚才复制的密码。
5. 在「OBS」页连接，开启「自动推流」。
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

### 弹幕与历史

登录或恢复已保存账号后，默认自动连接该账号的直播间；无需复制 Cookie、输入身份码或连接 OBS。按 `8` 打开「弹幕」页，切换其他页面不影响监听。

| 按键 | 操作 |
| --- | --- |
| `s` / `Enter` | 开关监听；关闭前确认，设置跨重启保存，已有历史不删除 |
| `Space` | 暂停显示 / 跟随最新；仅暂停显示时，后台监听和保存继续 |
| `[` / `]` | 更早 / 更新的历史页，每页最多 100 条记录 |
| `End` | 回到当前监听房间的最新记录 |
| `o` | 切换有本地历史的房间，无需登录即可浏览 |
| `f` | 显示 / 隐藏其他事件；原始事件始终保存，分页计数包含这些记录 |
| `r` | 刷新历史，故障时重试；不会主动断开健康连接 |
| `PgUp` / `PgDn` | 滚动并暂停跟随，避免新消息挤走正在阅读的内容 |

历史保存在数据目录的 `danmaku/history.db`，按直播间隔离。消息先通过同步数据库事务保存，再刷新界面；有可靠事件标识的消息跨重启去重，没有可靠标识的相同内容仍保留，避免误删真实重复弹幕。支持旧版及 protobuf 礼物消息；未知或格式异常的业务消息保留原始数据。超长文本仅截短显示，完整内容仍在本机历史中。

**可靠性边界：** 网页端不是保证投递的开放 API，没有可靠的离线重放游标。断网、关停应用、关闭监听、平台风控或服务端未下发时，可能缺失消息；不能承诺零丢失或用于完整财务对账。连接边界会写入历史。磁盘写入失败时会显示警告、保留当前消息并重试；切换账号前先处理待保存消息。退出时仍无法写入会报告错误，强制终止、断电及损坏的存储仍可能丢失未提交消息。

**本地隐私：** 聊天、礼物及原始业务事件不会自动清理，需要关注磁盘空间；SC 撤回后在界面隐藏，但原始数据仍留在本机。文件使用受限权限（Unix 目录 `0700`、数据库 `0600`），不等于加密。不要分享历史数据库。Cookie 和弹幕连接鉴权包不写入历史。一个数据目录同时只允许一个历史写入实例；被占用时弹幕页提示重试，不影响其他直播控制功能。

### 常用选项

界面默认跟随系统语言，也可以手动选择：

```sh
arcana-world --lang zh-CN               # 使用中文
arcana-world --lang en                  # 使用英文
arcana-world --config-dir /path/to/data # 指定数据目录
arcana-world --help                     # 查看帮助
```

默认数据目录：`~/.arcana/world`。

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

- [Radekyspec/StartLive](https://github.com/Radekyspec/StartLive)：本项目协议实现的来源。
- [xfgryujk/blivedm](https://github.com/xfgryujk/blivedm)：网页端弹幕协议、WBI 签名及事件结构参考（MIT）。
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)：终端界面框架。
