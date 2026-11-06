---
description: "通过编辑 YAML 模板文件，自定义浮层显示的文字和语音播报的措辞。"
---

# 自定义浮层与播报文字

浮层上每条消息的显示格式和语音播报的措辞均由模板文件定义。例如，让浮层只显示弹幕正文，或让播报说“谢谢某某送来的小心心”，修改一行模板即可。

模板只影响[弹幕浮层](/features/overlay)和[语音播报](/features/tts)的文字，弹幕列表不受影响。

## 模板文件位置 {#files}

模板位于配置目录的 `templates` 文件夹中，Arcana World 首次启动时自动生成：

- macOS、Linux：`~/.arcana/world/templates/`
- Windows：`%USERPROFILE%\.arcana\world\templates\`

| 用途 | 中文界面 | 英文界面 |
| --- | --- | --- |
| 浮层 | `overlay.zh-CN.yaml` | `overlay.en.yaml` |
| 语音播报 | `tts.zh-CN.yaml` | `tts.en.yaml` |

修改当前界面语言对应的文件。每份文件已包含所有事件的默认格式，每个事件上方的注释列出了可用字段，可在此基础上修改。

如果使用 `--config-dir` 指定了其他配置目录，模板位于该目录中，参见[数据目录与重置](/reference/data)。

## 修改与生效 {#edit}

1. 用文本编辑器打开模板文件。
2. 找到要修改的事件，如 `chat`（弹幕）或 `gift`（礼物），只修改冒号右侧引号内的内容。
3. 保存文件，然后重启 Arcana World。

修改需重启后生效。「示例预览」和「试听」使用固定文字，不经过模板，需通过真实弹幕查看效果。

注意：模板用单引号包裹时，内容中的单引号需写成两个 `''`。

## 示例 {#examples}

浮层文件（`overlay.zh-CN.yaml`）：

```yaml
# 只显示弹幕正文
chat: '{{.Text}}'
# 礼物和数量
gift: '{{.User}}赠送{{.Gift}}{{if gt .Count 0}} × {{.Count}}{{end}}{{.Coins}}'
```

播报文件（`tts.zh-CN.yaml`）：

```yaml
chat: '{{.User}}说：{{.Text}}'
gift: '谢谢{{.User}}送来的{{if gt .Count 0}}{{.Count}}个{{end}}{{.Gift}}'
sc: '{{.User}}发送了{{.Amount}}元醒目留言：{{.Text}}'
```

如需在浮层上显示时间，可写成：

```yaml
chat: '{{.Time.Format "15:04"}} {{.User}}：{{.Text}}'
```

`"15:04"` 是 Go 的时间格式写法，表示“时:分”；如需显示秒，写作 `"15:04:05"`。不支持 `HH:mm` 等写法。

## 常用字段 {#fields}

| 字段 | 内容 |
| --- | --- |
| `.User` | 用户名 |
| `.Text` | 弹幕或留言正文 |
| `.Gift` | 礼物名称 |
| `.Count` | 礼物数量 |
| `.Coins` | 礼物金额，带单位，例如「（100金瓜子）」 |
| `.Amount` | 醒目留言金额（元） |
| `.Time` | 收到的时间 |
| `.Guard` | 大航海等级，例如舰长、提督、总督 |

模板文件中每个事件上方的注释列出了该事件的其他可用字段。

## 模板错误处理 {#fallback}

模板有误时，应用会提示出错的文件，可定位时还会给出事件和行号。此时**整份文件**暂时改用内置的默认格式（浮层与播报互不影响），按提示修正并重启后即恢复。

如需恢复默认模板，删除该模板文件并重启，应用会重新生成默认文件。

:::: details 进阶：更多字段与语法
::: v-pre
模板采用 [Go text/template](https://pkg.go.dev/text/template) 的安全子集，支持：

- 字段：`{{.User}}`
- 条件：`{{if gt .Count 0}} × {{.Count}}{{end}}`
- `with`：`{{with .GuardDuration}}{{.}}的{{end}}`，在其中用 `{{$.User}}` 访问外层字段
- 函数：`and`、`or`、`not`、`len`、`eq`、`ne`、`lt`、`le`、`gt`、`ge`

不支持 `range`、变量、命名模板及其他函数。

更多字段：`.CoinType`、`.GuardDuration`、`.Silent`、`.UID`、`.RoomID`、`.MedalName`、`.MedalLevel`、`.Title` 等，并非所有事件都提供。部分事件的数据需通过 `{{.F "data.username" "观众"}}`（文字，第二个参数为缺失时的替代值）或 `{{.N "data.gift_num"}}`（数字）读取，可用字段名以模板文件中的注释为准。

礼物的 `.Amount` 为金瓜子或银瓜子数量，而非人民币金额。
:::
::::
