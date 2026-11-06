---
description: "Arcana World 的命令行参数：选择启动界面、临时覆盖设置、诊断与检查更新。"
---

# 命令行参数

日常开播无需任何参数，直接运行程序即可。以下参数用于切换启动界面、临时覆盖设置或排查问题。

## 使用方法 {#how-to-run}

在解压后的文件夹中打开终端，在程序名后添加参数：

::: code-group

```sh [macOS / Linux]
./arcana-world --gui
```

```powershell [Windows]
.\arcana-world.exe --gui
```

:::

通过 APT 或 Scoop 安装时直接输入 `arcana-world`，下文均采用此写法。关闭开关类参数时写作 `--tts=false`，须用 `=` 连接，不能用空格分隔。

常用参数：

```sh
arcana-world --gui            # 本次以网页面板启动
arcana-world --default-gui    # 将网页面板设为默认界面
arcana-world --log-level debug  # 排查问题时记录详细日志
arcana-world --doctor         # 检查配置和辅助程序
```

## 启动界面 {#launch}

| 参数 | 作用 |
| --- | --- |
| `--tui` | 本次以终端界面启动。 |
| `--gui` | 本次以网页面板启动并打开浏览器（默认 `http://127.0.0.1:5273`）。 |
| `--default-tui` | 将终端界面设为默认界面，保存后退出。 |
| `--default-gui` | 将网页面板设为默认界面，保存后退出。 |
| `--web-port 端口` | 网页面板端口，默认 5273；`0` 表示自动选择空闲端口。 |

未设置默认界面时，程序以终端界面启动。在 Windows 上双击 `arcana-world-webui.exe` 等同于运行 `arcana-world.exe --gui`。

## 本次运行的临时设置 {#overrides}

以下参数仅影响本次运行，不修改已保存的设置；被覆盖的设置项在界面中显示为只读。

| 参数 | 作用 |
| --- | --- |
| `--lang 语言` | 界面语言：`auto`（跟随系统）、`zh-CN`、`en`。 |
| `--obs-auto-connect` | 自动连接 OBS；`=false` 关闭。 |
| `--obs-auto-stream` | 开播时自动启动 OBS 推流；`=false` 关闭。 |
| `--tts` | 开启语音播报；`=false` 关闭。 |
| `--overlay` | 开启已选的浮层样式；`=false` 关闭。 |
| `--overlay-executable 路径` | 使用指定的浮层辅助程序。 |
| `--proxy 地址` | 代理：HTTP(S) 或 SOCKS5 地址，`direct` 表示不使用代理。 |
| `--websocket-addr IP:端口` | 在指定地址开启 [WebSocket 弹幕广播](/features/websocket)。 |

## 数据与日志 {#data}

| 参数 | 作用 |
| --- | --- |
| `--config-dir 目录` | 指定数据目录，默认 `~/.arcana/world`。 |
| `--credential-backend 方式` | 登录信息和密码的保存方式：`auto`（默认）、`system`、`file`、`memory`，区别见[数据目录与重置](/reference/data#credentials)。 |
| `--log-level 级别` | 日志级别：`debug`、`info`（默认）、`warn`、`error`。 |

## 诊断与更新 {#diagnostics}

| 参数 | 作用 |
| --- | --- |
| `--doctor` | 只读检查配置、凭据存储和辅助程序，不启动界面。 |
| `--check-update` | 检查 GitHub 上是否有新的稳定版，不自动下载。 |
| `--version` | 显示版本号。 |
| `--help`、`-h` | 显示帮助；`--lang en --help` 显示英文帮助。 |

`--doctor` 的每行输出以 `[OK]`、`[WARN]` 或 `[ERROR]` 开头，仅 `[ERROR]` 需要处理。排查浮层或语音问题时加上 `--overlay` 或 `--tts`，相关辅助程序缺失时显示为 `[ERROR]`。

::: details 参数组合限制
参数组合错误时，程序报错并退出：

- `--tui`、`--gui`、`--default-tui`、`--default-gui`、`--doctor`、`--check-update` 每次只能使用一个。
- `--default-tui` / `--default-gui` 只能与 `--config-dir`、`--lang`、`--credential-backend` 搭配。
- `--web-port` 仅在以网页面板启动时可用。
- 不接受位置参数，例如 `--tts false` 须写作 `--tts=false`。

指定 `--help` 或 `--version` 时，其他参数均被忽略。
:::
