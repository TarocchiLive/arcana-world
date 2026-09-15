
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

<p align="center">
  <img src="assets/arcana-world-1-cn.png" alt="主菜单" width="48%">
  <img src="assets/arcana-world-2-cn.png" alt="封面图预览" width="48%">
</p>

## 功能详情

- 扫码登录，保存和切换多个账号。
- 开播、停播，修改标题、分区、公告和封面。
- 自动裁剪封面，支持终端图片预览（需要终端模拟器支持，建议使用 kitty 或 ghostty ）后再上传。
- 将推流配置写入 OBS，按需同步开停播。
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
| `Tab` / `1–7` | 切换页面 |
| `PgUp` / `PgDn` | 滚动 |
| `r` | 刷新 |
| `Esc` | 返回或取消 |
| `q` | 退出 |

### 常用选项

界面默认跟随系统语言，也可以手动选择：

```sh
arcana-world --lang zh-CN               # 使用中文
arcana-world --lang en                  # 使用英文
arcana-world --config-dir /path/to/data # 指定数据目录
arcana-world --help                     # 查看帮助
```

默认数据目录：`~/.arcana/world`。

### 桌面浮层（弹幕姬）

支持无焦点，鼠标穿透，半透明实时显示弹幕列表，支持跨平台。

详见 [Arcana Overlay](cmd/arcana-overlay/README.md)。

## TODO
- [ ] 支持获取b站推流直链，自动连接 OBS，自动开播。
- [ ] 支持b站直播间弹幕抓取，提供 TUI 内礼物，聊天记录浏览。
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

- [Radekyspec/StartLive](https://github.com/Radekyspec/StartLive)：本项目协议实现的来源。
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)：终端界面框架。
