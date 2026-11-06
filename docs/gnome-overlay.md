# GNOME 精美浮层

GNOME Wayland 不提供 layer-shell 协议，精美浮层需要配套的 GNOME Shell 扩展才能固定位置并保持在其他窗口之上。扩展只管理窗口的位置和层级，画面和动效与其他桌面一致。

原生浮层不支持 GNOME Wayland，两种浮层都不支持 X11 会话。

返回 [Arcana World Overlay](../cmd/arcana-world-overlay/README.md)。

## 适用版本

扩展支持 GNOME 45 至 50，其中 48.7 已实测。其余版本对照 [GNOME 扩展迁移文档](https://gjs.guide/extensions/upgrading/gnome-shell-45.html)和 Mutter 接口核对，未发现影响本扩展的变化。GNOME 44 及更早版本的扩展格式不同，不受支持。

## 安装

扩展位于 Linux 发行包和源码仓库的 `extensions/gnome/arcana-overlay@arcana-world`。在解压目录或项目根目录执行：

```sh
dest="${XDG_DATA_HOME:-$HOME/.local/share}/gnome-shell/extensions/arcana-overlay@arcana-world"
mkdir -p "$dest"
cp extensions/gnome/arcana-overlay@arcana-world/{metadata.json,extension.js} "$dest/"
```

注销并重新登录 GNOME，然后启用扩展：

```sh
gnome-extensions enable arcana-overlay@arcana-world
```

之后在「弹幕浮层」页选择「精美浮层」并打开「浮层」开关即可。

## 使用说明

- 浮层最终位置由 GNOME 决定，与顶栏重叠时可能被移到顶栏下方。
- 同一时间只能运行一个精美浮层。
- 锁屏或禁用扩展会关闭浮层。解锁或重新启用扩展后，关闭再打开「浮层」开关。

## 更新与卸载

更新 Arcana World 后，重新执行安装中的复制命令，再注销并重新登录。

卸载扩展：

```sh
gnome-extensions uninstall arcana-overlay@arcana-world
```

## 常见问题

错误信息显示在「弹幕浮层」页和「日志」页。

| 错误信息包含 | 处理 |
| --- | --- |
| `enable the Arcana Elegant GNOME Shell extension` | 扩展没有运行。确认已安装并重新登录，再执行启用命令 |
| `An overlay owner is already active` | 已有一个精美浮层在运行，先关闭它 |
| `extension disabled or service owner changed`、`extension disconnected` | 锁屏或扩展被禁用。恢复后关闭再打开「浮层」开关 |
| `initial placement timed out`、`Window placement timed out` | 浮层窗口 5 秒内没有显示。关闭再打开「浮层」开关重试 |

## 从源码构建

用 `make overlay-qt` 构建精美浮层，Linux 需要 Qt DBus 开发组件，其余依赖见[从源码构建](building.md#浮层)。扩展是 JavaScript 源码，无需编译。
