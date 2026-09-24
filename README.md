# Arcana World

<p align="center">
  <img src="assets/arcana-world.webp" alt="Arcana World 世界塔罗牌图标" width="180">
</p>

[![CI](https://github.com/TarocchiLive/arcana-world/actions/workflows/ci.yml/badge.svg)](https://github.com/TarocchiLive/arcana-world/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.26%2B-00ADD8?logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/license-GPL--3.0--only-blue)](LICENSE)
[![Release](https://img.shields.io/github/v/release/TarocchiLive/arcana-world?sort=semver&cacheSeconds=300)](https://github.com/TarocchiLive/arcana-world/releases/latest)
[![QQ](https://img.shields.io/badge/Join-QQ_Group-ff69b4)](https://qm.qq.com/q/TkCteIVlOo)

**中文** · [English](README.en.md)

Arcana World 是一款基于 Go 的 bilibili 直播姬 + 弹幕姬。

支持手机扫码登录，弹幕悬浮窗，语音播报。跨平台。

原生支持Windows、Linux、MacOS。

## 预览

<p align="center">
  <img src="assets/arcana-world-01.webp" alt="主菜单" width="48%">
  <img src="assets/arcana-world-02.webp" alt="浮层配置项" width="48%">
  <img src="assets/arcana-world-03.webp" alt="封面图预览" width="48%">
  <img src="assets/arcana-world-04.webp" alt="弹幕浮层" width="48%">
</p>

## 功能亮点

- **扫码登录，连接 OBS，然后一键开播**
- **直播间实时弹幕收发，提供穿透弹幕悬浮窗，不干扰鼠标键盘操作**
- **支持弹幕，礼物语音播报，SC播报，基于 Edge TTS，0 占用，高质量**
- **常态内存占用 40MB，超轻量、超流畅、超稳定**

## 安装

### 快速开始

从 [最新版本](https://github.com/TarocchiLive/arcana-world/releases/latest) 下载对应平台的包，如 arcana-world-windows-amd64.zip，完整解压后运行根目录的 `arcana-world`（Windows 为 `arcana-world.exe`）即可。

### 从源码构建

从源码构建需要 Go 1.26.8+ 和对应平台的[浮层构建依赖](cmd/arcana-world-overlay/README.md#构建与启动)。Linux 下还需安装 ALSA 开发库与 `pkg-config`，用于构建配套音频程序（Debian/Ubuntu：`libasound2-dev pkg-config`）：

```sh
git clone https://github.com/TarocchiLive/arcana-world.git
cd arcana-world
make desktop
./bin/arcana-world
```

构建后运行 `bin/arcana-world`（Windows 为 `bin/arcana-world.exe`）。只需要终端功能时（无浮层），可使用 `make build`，无需安装图形开发库。浮层的平台支持情况见 [Arcana World Overlay](cmd/arcana-world-overlay/README.md#支持的平台)。

## 使用帮助

在软件中按 `0` 或打开「帮助」查看操作说明和常见问题；命令行选项见 `arcana-world --help`。

## TODO
- [x] 支持获取 B 站推流直链，自动连接 OBS 并开播。
- [x] 支持 B 站直播间弹幕抓取，并在 TUI 中浏览礼物和聊天记录。
- [x] 支持 B 站直播间弹幕浮层。
- [x] 支持 TTS 朗读弹幕和礼物播报。
- [x] 多显示器浮层热切换。
- [x] 鼠标 tui 操作。
- [x] tui 界面重构美化。
- [x] 在终端发送弹幕。
- [ ] 浮层界面重构美化。
- [ ] 支持 iterm2 图片协议。
- [ ] 终端缩放裁剪封面图。
- [ ] 版本更新检查。
- [ ] Scoop、Homebrew、nixpkgs 等软件仓库支持。
- [ ] 机器人协议接口。
- [?] GUI / WebUI 支持（待定）。

## 开发

```sh
make run    # 本地运行
make check  # 测试与静态检查
make build  # 构建
```

翻译词表位于 [`internal/i18n/locales`](internal/i18n/locales)，按语言和模块划分。欢迎通过 [Issue](https://github.com/TarocchiLive/arcana-world/issues) 或 Pull Request 反馈问题、参与改进。

## 交流与反馈

<img src="assets/qq_group.webp" alt="QQ群" width="33%">

## 声明

本项目是独立开发的开源工具。使用者应自行遵守适用的法律法规和平台规则。本项目仅用于学习和交流 Go 及相关框架，请勿用于其他用途。


## 许可证

采用 [GPL-3.0-only](LICENSE)。

## 致谢

- [Radekyspec/StartLive](https://github.com/Radekyspec/StartLive)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [Bubbles](https://github.com/charmbracelet/bubbles)
- [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- [edge-tts](https://github.com/rany2/edge-tts)
- [edge-tts-go](https://github.com/wujunwei928/edge-tts-go)
