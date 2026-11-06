---
description: "开启 WebSocket 弹幕广播，在 OBS 浏览器源或自定义脚本中实时接收弹幕、礼物等直播间事件。"
---

# WebSocket 弹幕广播

开启广播后，Arcana World 会将收到的弹幕、礼物等事件实时推送给本机的 WebSocket 客户端，可用于在 OBS 浏览器源中实现自定义弹幕样式，或通过脚本处理直播间事件。

仅需在屏幕上显示弹幕时，使用内置的[弹幕浮层](/features/overlay)。

广播设置目前仅在终端界面中提供。使用网页面板的用户，可通过 `arcana-world --tui` 打开终端界面完成设置，之后通过网页面板启动时广播同样会开启。

## 开启广播 {#enable}

1. 在终端界面按 `8` 打开「设置」，选择「WebSocket 广播」进入详情。
2. 选择「WebSocket 访问令牌」，输入一串随机长字符串，按 `Enter` 保存。强烈建议设置令牌，原因参见[令牌与安全](#security)。
3. 选择「WebSocket 广播」，按 `Enter` 开启。

![终端界面「设置」页](/screenshots/tui-settings.webp)

「WebSocket 广播」一行显示「运行中」即表示已开启，此后每次启动都会自动开启。

默认地址为 `127.0.0.1:7391`，仅允许本机连接。显示「启动失败」通常表示端口被占用，在「WebSocket 监听地址」中更换端口（如 `127.0.0.1:7392`）后重试。

## 连接 {#url}

客户端连接以下地址后即开始接收事件，无需发送订阅消息：

```text
ws://127.0.0.1:7391/ws?token=你的令牌
```

未设置令牌时去掉 `?token=…`。令牌包含 `&`、`#`、空格等字符时需进行 URL 编码（JavaScript 中使用 `encodeURIComponent`）。

## 示例：在 OBS 中显示弹幕 {#example}

以下网页可直接使用：显示最近 20 条弹幕、礼物和醒目留言，断线后每 3 秒自动重连。

1. 将代码保存为 `chat.html`，将 `TOKEN` 改为你的令牌（未设置令牌时写 `""`）。
2. 在 OBS 中添加「浏览器」来源，勾选「本地文件」，选择 `chat.html`。
3. 在直播间发送一条弹幕，确认来源中是否显示。

```html
<!doctype html>
<meta charset="utf-8">
<style>
  body { margin: 0; font: 24px/1.5 sans-serif; color: #fff; }
  #log div { text-shadow: 0 0 4px #000; }
</style>
<div id="log"></div>
<script>
  const ADDR = "127.0.0.1:7391";
  const TOKEN = "你的令牌"; // 未设置令牌时写 ""
  const log = document.getElementById("log");

  function show(text) {
    const line = document.createElement("div");
    line.textContent = text; // 使用 textContent，避免观众发送的内容被解析为网页代码
    log.append(line);
    while (log.childElementCount > 20) log.firstElementChild.remove();
  }

  function describe(event) {
    const cmd = String(event.cmd || "").split(":")[0];
    const data = event.data || {};
    if (cmd === "DANMU_MSG" && Array.isArray(event.info)) {
      const user = Array.isArray(event.info[2]) ? event.info[2][1] : "";
      return `${user}：${event.info[1]}`;
    }
    if (cmd === "SEND_GIFT") return `${data.uname} 赠送 ${data.giftName} ×${data.num}`;
    if (cmd === "SUPER_CHAT_MESSAGE") {
      return `[SC ¥${data.price}] ${data.user_info?.uname}：${data.message}`;
    }
    return null; // 忽略其他事件
  }

  function connect() {
    let url = `ws://${ADDR}/ws`;
    if (TOKEN) url += "?token=" + encodeURIComponent(TOKEN);
    const ws = new WebSocket(url);
    ws.onmessage = (message) => {
      try {
        const text = describe(JSON.parse(message.data).event || {});
        if (text) show(text);
      } catch (_) {}
    };
    ws.onclose = () => setTimeout(connect, 3000);
  }
  connect();
</script>
```

未显示时，依次检查：

1. 「WebSocket 广播」显示「运行中」，地址和端口与示例一致。
2. 令牌一致。令牌错误时连接会被拒绝。
3. 弹幕监听已开启，「弹幕」页能看到该弹幕。
4. 该弹幕未被「全局屏蔽词」过滤。

## 消息格式 {#format}

每条消息是一个 JSON 对象：`room_id` 为直播间 ID，`event` 为 B 站下发的原始事件，字段原样保留。

```json
{"room_id":12345,"event":{"cmd":"DANMU_MSG","info":[[],"你好",[67890,"用户",0]]}}
```

`event.cmd` 为事件类型，可能带后缀（如 `DANMU_MSG:4:0:2:2:2:0`），判断时取第一个冒号前的部分。常用事件：

| 事件 | `cmd` | 常用字段 |
| --- | --- | --- |
| 弹幕 | `DANMU_MSG` | `event.info[1]` 正文；`event.info[2][1]` 昵称 |
| 礼物 | `SEND_GIFT` | `event.data.uname` 昵称；`event.data.giftName` 礼物名；`event.data.num` 数量 |
| 醒目留言 | `SUPER_CHAT_MESSAGE` | `event.data.message` 正文；`event.data.price` 金额（元）；`event.data.user_info.uname` 昵称 |

这些字段来自 B 站协议，可能变化或缺失，脚本中应先判断字段是否存在。

转发的事件包括弹幕、礼物、醒目留言、上舰、入场、关注、点赞、开播下播、直播间标题变更、警告和禁言等；不转发统计和榜单类事件。包含「全局屏蔽词」的事件整条不转发，「播报屏蔽词」不影响广播，参见[屏蔽词](/features/blocklist)。

## 令牌与安全 {#security}

广播允许任意网页跨源连接，以便 OBS 浏览器源使用。因此未设置令牌时，浏览器中打开的任何网页都能读取广播内容。请务必设置令牌，并将带令牌的地址视同密码，不要分享，也不要出现在直播画面中。

::: warning 允许局域网内其他电脑连接
将监听地址改为 `0.0.0.0:7391` 或本机局域网 IP，其他电脑连接 `ws://本机局域网IP:7391/ws?token=…`。此设置会向所有能访问该端口的设备开放，且广播不加密，务必先设置随机长令牌；如需加密，请自行配置可信的 TLS 反向代理。
:::

::: details 更多细节
- **只读**：广播仅向外发送事件，不接受任何指令；客户端发送消息会被断开连接。
- **断线**：断线期间的事件不会补发，客户端需自行重连（上述示例已实现）。处理过慢、积压过多的客户端也会被断开。
- **修改地址或令牌**：已有连接会断开，客户端使用新的地址或令牌重新连接即可。
- **请求头**：支持自定义请求头的客户端，可改用 `Authorization: Bearer 你的令牌`。
- **令牌存储**：令牌保存在应用的凭据存储中，不写入 `config.json`。「恢复默认设置」保留令牌，「清除应用数据」会删除令牌。提交空值可关闭验证。
- **仅本次运行开启**：`arcana-world --websocket-addr 127.0.0.1:7391` 会在本次运行中以该地址开启广播，不修改已保存的设置，参见[命令行参数](/reference/cli)。
- **地址格式**：`IP:端口`，例如 `127.0.0.1:7391`、`[::1]:7391`、`localhost:7391`。
:::
