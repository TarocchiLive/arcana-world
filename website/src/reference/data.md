---
description: "Arcana World 的数据存储位置，以及恢复默认设置、清除数据、备份和迁移到新电脑的方法。"
---

# 数据目录与重置

## 数据位置 {#location}

设置、弹幕历史和日志保存在用户目录下的 `.arcana/world` 中：

| 系统 | 位置 |
| --- | --- |
| macOS | `/Users/用户名/.arcana/world` |
| Linux | `/home/用户名/.arcana/world` |
| Windows | `C:\Users\用户名\.arcana\world` |

以 `.` 开头的文件夹在 macOS 访达中默认隐藏，按 `Cmd+Shift+.` 显示。如需使用其他位置，启动时添加 `--config-dir 目录`。

目录中的主要内容：

- `config.json`：所有设置、账号列表和屏蔽词。
- `templates/`：浮层和语音播报的文字模板，修改方法见[自定义浮层与播报文字](/features/templates)。
- `logs/arcana-world.log`：运行日志，用于排查问题，见[常见问题](/reference/troubleshooting#logs)。
- `danmaku/history.db`：最近 7 天的弹幕历史。

本地语音模型不在此目录中，位于程序所在文件夹的 `models/tts/` 下。

## 登录信息位置 {#credentials}

B 站登录信息、OBS 密码和 API Key 不写入 `config.json`，默认保存在系统密码库中（macOS 钥匙串、Windows 凭据管理器、Linux Secret Service）。Linux 上没有 Secret Service 时，改为保存到数据目录的 `credentials/secrets.json`。该文件为明文，请勿发送给他人。

::: details 更改保存方式
启动时通过 `--credential-backend` 指定：

- `auto`（默认）：即上述行为。
- `system`：仅使用系统密码库，不可用时报错。
- `file`：保存到 `credentials/secrets.json`，明文。
- `memory`：仅保存在内存中，退出后丢失，下次启动需重新登录。

更改保存方式后，已有的登录信息不会迁移，需要重新扫码登录，并重新填写 OBS 密码和 API Key。
:::

## 恢复默认设置与清除应用数据 {#reset}

两个按钮均位于设置页：终端界面按 `8` 打开「设置」；网页面板位于「设置」→「维护」。

| | 恢复默认设置 | 清除应用数据 |
| --- | --- | --- |
| 适用场景 | 设置出现混乱，需要回到初始状态 | 不再使用，或需要完全重新开始 |
| 设置（代理、浮层、语音、外观等） | 恢复默认 | 删除 |
| 登录信息、OBS 密码、屏蔽词 | 保留 | 删除 |
| 弹幕历史、日志、模板文件 | 保留 | 删除 |
| 本地语音模型 | 保留 | 保留 |
| 执行后 | 立即生效，继续使用 | 程序退出，下次启动需重新登录 |

清除应用数据时需输入 `arcanaworldclear` 确认。该操作会同时删除修改过的模板；如需保留，请先复制 `templates` 文件夹。

遇到问题时，建议先查阅[常见问题](/reference/troubleshooting)，再考虑清除数据。

## 备份与迁移 {#backup}

1. 完全退出 Arcana World（以网页面板方式运行时，在终端按 `Ctrl+C`）。
2. 复制整个数据目录。
3. 在新电脑上将其放回 `~/.arcana/world`，或启动时用 `--config-dir` 指定该目录。

登录信息默认保存在系统密码库中，不随文件夹迁移，因此在新电脑上需要重新扫码登录，并重新填写 OBS 密码和 API Key。备份包含设置和代理地址，请存放在仅自己可访问的位置。

升级程序不会修改数据目录。如果将新版本解压到其他文件夹，将旧文件夹中的 `models/tts/` 一并复制过去，即可免去重新下载语音模型。
