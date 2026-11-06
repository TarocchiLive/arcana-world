---
description: "在 Windows、macOS 和 Linux 上安装、启动、升级和卸载 Arcana World。"
---

# 安装与升级

## 安装 {#install}

从 [GitHub 最新版本](https://github.com/TarocchiLive/arcana-world/releases/latest)下载对应系统的压缩包，例如 Windows 选择 `arcana-world-windows-amd64.zip`，Apple 芯片的 Mac 选择 `arcana-world-darwin-arm64.tar.xz`。ARM 设备将 `amd64` 替换为 `arm64`。

压缩包内没有外层文件夹，请先新建文件夹再解压到其中。

::: code-group

```text [Windows]
1. 新建文件夹，例如 D:\arcana-world。
2. 右键压缩包 →「全部解压缩」，解压到该文件夹。
3. 双击 arcana-world-webui.exe 打开网页面板，
   或双击 arcana-world.exe 打开终端界面。
```

```sh [macOS]
mkdir -p ~/arcana-world
tar -xf ~/Downloads/arcana-world-darwin-arm64.tar.xz -C ~/arcana-world
cd ~/arcana-world
xattr -dr com.apple.quarantine .
./arcana-world
```

```sh [Debian / Ubuntu]
sudo apt-get update
sudo apt-get install -y ca-certificates curl
sudo install -d -m 0755 /etc/apt/keyrings
curl -fsSL https://tarocchilive.github.io/apt/tarocchilive-archive-keyring.gpg | sudo tee /etc/apt/keyrings/tarocchilive.gpg >/dev/null
sudo chmod 0644 /etc/apt/keyrings/tarocchilive.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/tarocchilive.gpg] https://tarocchilive.github.io/apt stable main" | sudo tee /etc/apt/sources.list.d/tarocchilive.list
sudo apt-get update
sudo apt-get install arcana-world
arcana-world
```

```sh [其他 Linux]
mkdir -p ~/arcana-world
tar -xf ~/Downloads/arcana-world-linux-amd64.tar.xz -C ~/arcana-world
cd ~/arcana-world
./arcana-world
```

:::

Windows 10 用户使用终端界面前，请先阅读[终端入门](/guide/terminal)，否则可能出现乱码。

::: details 用 Scoop 或 Nix 安装
Windows 上使用 Scoop：

```powershell
scoop bucket add tarocchilive https://github.com/TarocchiLive/scoop-bucket
scoop install tarocchilive/arcana-world
```

NixOS 或已安装 Nix 的 Linux / macOS 可从源码构建：

```sh
git clone https://github.com/TarocchiLive/arcana-world.git
cd arcana-world
nix build
./result/bin/arcana-world
```

Nix 构建的程序位于只读的 `/nix/store` 中，无法安装本地语音模型，云端语音可正常使用。
:::

### macOS：放行下载的程序 {#macos-quarantine}

macOS 会为浏览器下载的程序添加隔离标记并阻止其运行。上文的 `xattr -dr com.apple.quarantine .` 会在解压后的文件夹中放行整套程序，包括浮层和语音辅助程序。仅在「系统设置」中允许主程序不够，语音播报可能没有声音。

每次升级替换文件后，都需要在新文件夹中重新运行该命令。

## 首次启动 {#first-launch}

不带参数启动时，程序打开终端界面，首页为「直播」页。使用网页面板时，Windows 双击 `arcana-world-webui.exe`，其他系统运行：

```sh
./arcana-world --gui
```

程序会自动打开浏览器；如未打开，请手动访问 `http://127.0.0.1:5273`。

随后按快速上手完成第一次开播：[网页面板](/guide/start-web)或[终端界面](/guide/start-tui)。

## 升级 {#update}

有新版本时，终端界面显示「↑ 新版本」和版本号，网页面板侧栏顶部显示发布页链接。

::: code-group

```text [压缩包]
1. 退出 Arcana World。
2. 下载新版压缩包，解压并覆盖原文件夹。
3. macOS 需在文件夹中重新运行：
   xattr -dr com.apple.quarantine .
```

```sh [APT]
sudo apt-get update
sudo apt-get install --only-upgrade arcana-world
```

```powershell [Scoop]
scoop update arcana-world
```

```sh [Nix]
cd arcana-world
git pull
nix build
```

:::

设置、账号和弹幕历史保存在数据目录中，升级不会影响这些数据。

## 卸载 {#uninstall}

压缩包版直接删除整个程序文件夹；APT 使用 `sudo apt-get remove arcana-world`，Scoop 使用 `scoop uninstall arcana-world`。

卸载不会删除设置和账号，重新安装后仍可使用。如需同时清除数据，请先在程序的「设置」中清除应用数据再卸载，详见[数据目录与重置](/reference/data)。
