---
description: "参与 Arcana World 开发：本地构建、提交规范、翻译与反馈渠道。"
---

# 参与开发

欢迎提交错误修正、翻译和新功能。

## 本地构建 {#make}

构建终端主程序只需安装 Go（版本见 `go.mod`）：

```sh
git clone https://github.com/TarocchiLive/arcana-world.git
cd arcana-world
make build
./bin/arcana-world
```

以下常用命令均在项目根目录运行：

| 命令 | 作用 |
| --- | --- |
| `make run` | 直接运行 |
| `make check` | 测试和静态检查，提交前运行 |
| `make desktop` | 完整桌面包，含浮层和语音辅助程序 |
| `make clean` | 清理 `bin/` |

NixOS 可使用 `nix build`。浮层和语音辅助程序需要额外依赖（图形库、Qt、Python 等），各平台要求见[从源码构建](https://github.com/TarocchiLive/arcana-world/blob/main/docs/building.md)；代码组织方式见[架构概览](https://github.com/TarocchiLive/arcana-world/blob/main/docs/architecture.md)。

## 提交规范 {#commits}

提交信息使用英文，格式为 `type(scope): subject`，例如：

```text
fix(overlay): let taller panels show more chat messages
```

`type` 取值为 `feat`、`fix`、`docs`、`refactor`、`perf`、`style`、`test`、`build`、`ci`、`chore`、`revert`、`release`、`merge` 之一。代码注释使用中文。其他约定见仓库根目录的 [AGENTS.md](https://github.com/TarocchiLive/arcana-world/blob/main/AGENTS.md)。

## 翻译 {#translations}

界面文字位于 [`internal/i18n/locales`](https://github.com/TarocchiLive/arcana-world/tree/main/internal/i18n/locales)，按 `zh-CN` 和 `en` 分别存放。修改文字时须同时更新中英文。

## 反馈与交流 {#feedback}

- 问题与建议：提交到 [GitHub Issues](https://github.com/TarocchiLive/arcana-world/issues)，需附带的信息见[常见问题](/reference/troubleshooting#report)。
- 代码修改：直接提交 Pull Request。
- 交流讨论：扫码或[点击此处](https://qm.qq.com/q/TkCteIVlOo)加入 QQ 群。

![Arcana World QQ 交流群二维码](/images/qq_group.webp)

## 许可证与致谢 {#license}

Arcana World 采用 [GPL-3.0-only](https://github.com/TarocchiLive/arcana-world/blob/main/LICENSE) 许可证。

本项目是独立开发的开源工具。使用者应自行遵守适用的法律法规和平台规则。本项目仅用于学习和交流 Go 及相关框架，请勿用于其他用途。

感谢以下项目：

- [Radekyspec/StartLive](https://github.com/Radekyspec/StartLive)
- [Bubble Tea](https://github.com/charmbracelet/bubbletea)
- [Kotonoha](https://github.com/locez/kotonoha)
- [Bubbles](https://github.com/charmbracelet/bubbles)
- [Lip Gloss](https://github.com/charmbracelet/lipgloss)
- [edge-tts](https://github.com/rany2/edge-tts)
- [Haivision SRT](https://github.com/Haivision/srt)
- [sherpa-onnx](https://github.com/k2-fsa/sherpa-onnx)、[ONNX Runtime](https://github.com/microsoft/onnxruntime)、[MeloTTS](https://github.com/myshell-ai/MeloTTS)
- [Kokoro](https://huggingface.co/hexgrad/Kokoro-82M-v1.1-zh)、[Piper](https://github.com/rhasspy/piper)、[LJ Speech](https://keithito.com/LJ-Speech-Dataset/)、[eSpeak NG](https://github.com/espeak-ng/espeak-ng)
