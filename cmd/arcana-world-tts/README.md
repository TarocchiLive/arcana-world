# Arcana World 语音播报

**中文** · [English](README.en.md)

把弹幕、礼物和醒目留言读出来。可以用在线语音，也可以用装在电脑上的本地语音。

[返回 Arcana World](../../README.md)

## 快速开始

1. 在「TTS」页选择语音供应商，打开它的设置。WebUI 在「输出 → 声音」。
2. 选择音色。云端语音填写 API Key，本地语音安装模型。
3. 点「试听当前音色」（WebUI 为「试听语音」），确认能听到声音。
4. 在「选择语音播报事件」（WebUI 为「语音事件」）里勾选要读的内容，再打开语音播报。

声音从运行 Arcana World 的电脑播放。每个供应商的设置单独保存，换回来不用重新填写。发行包请完整解压，主程序旁的 `libexec/` 文件夹不能删，升级时一起替换。

## 选哪种语音

| 语音 | 特点 | 需要准备 |
| --- | --- | --- |
| Edge | 免费，多种中文音色，偶尔会暂时连不上 | 联网 |
| 千问 | 付费云端语音，稳定 | 阿里云百炼 API Key |
| 豆包 | 付费云端语音，稳定，音色多 | 火山引擎 API Key |
| MeloTTS | 本地，中英双语，一个女声 | 下载约 164 MB 模型 |
| Kokoro v1.1 | 本地，中英双语，103 个音色 | 下载约 365 MB 模型 |
| Piper | 本地，只说英语，美式女声 | 下载约 67 MB 模型 |

本地语音装好后不用联网，但合成时会占用电脑性能，玩游戏直播时留意。

## 千问和豆包

**千问**：在[阿里云百炼](https://www.alibabacloud.com/help/en/model-studio/qwen-tts-api)开通语音合成，创建 API Key 后填入设置。默认服务地址对应中国内地，国际站账号请改成所在地域的地址。模型、音色和语言已有默认值，按需修改。

**豆包**：在[火山引擎](https://www.volcengine.com/docs/6561/1598757)开通语音合成，获取 API Key 后填入设置。默认音色是 Vivi 2.0；要换音色，在设置里填写已开通的资源 ID 和[音色 ID](https://www.volcengine.com/docs/6561/1257544)。

API Key 保存后不再显示，要更换时直接重新填写。「恢复默认设置」会保留 API Key，「清除应用数据」会删除它。网络抖动或服务繁忙时会自动重试两次，仍不成功才提示错误。

## 本地模型

第一次选中还没安装的模型时，会询问是否下载；也可以在设置里点「安装模型」。下载可以随时取消，下次会接着下，下载完会自动校验后再安装。

模型放在主程序旁的 `models/tts/` 文件夹，所以程序要放在可以写入的位置。所有用户数据目录共用这些模型，「清除应用数据」和「恢复默认设置」都不会删除它们。

### 手动安装

应用里下载太慢时，可以自己下载模型包：

| 模型 | 下载 | 模型文件夹（在 `models/tts/` 里） |
| --- | --- | --- |
| MeloTTS | [ZIP](https://dl.hdslb.com/bili/bililive/win/sherpa-onnx-shared-resources/27d5522a87dd49aa7f34f01fd5d9f9ee.zip) | `melo-zh-en/` |
| Kokoro v1.1 | [tar.bz2](https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/kokoro-multi-lang-v1_1.tar.bz2) | `kokoro-multi-lang-v1_1/` |
| Piper | [tar.bz2](https://github.com/k2-fsa/sherpa-onnx/releases/download/tts-models/vits-piper-en_US-ljspeech-medium.tar.bz2) | `vits-piper-en_US-ljspeech-medium/` |

两种方式任选：

- 把下载的压缩包放到设置里显示的压缩包路径，点「安装模型」。
- 把压缩包里的全部文件解压到设置里显示的模型文件夹，点「校验模型」。文件要直接放在这个文件夹里，不要多套一层目录。

## GPU 还是 CPU

本地语音默认用显卡合成。提示 GPU 无法启动时，在设置里把「运行设备」改成 CPU。也可以在设置里调整 CPU 线程数和语速。

| 系统 | 能否用 GPU |
| --- | --- |
| macOS | Apple 芯片，macOS 14 及以上 |
| Windows | x64、ARM64 |
| Linux | x64、ARM64 可以，需要系统安装支持 Vulkan 的显卡驱动；发行包自带 Vulkan 加载器 |

NixOS 会自动发现系统配置的显卡驱动，无需手动指定 NVIDIA、AMD 或 Intel 的驱动文件。

## 常见问题

| 现象 | 怎么办 |
| --- | --- |
| 试听没有声音 | 检查这台电脑的输出设备、系统音量和 TTS 音量 |
| 试听正常，直播时不读 | 检查语音播报开关、弹幕监听和事件选择；只会读打开播报后收到的新事件 |
| 提示 API Key 不正确或服务未开通 | 重新填写 API Key，并确认账号已开通语音合成和所选音色 |
| 提示无法连接 | 检查网络和代理；千问、豆包还要检查服务地址 |
| 模型校验失败 | 重新安装；手动安装时确认用的是上表中的完整模型包，文件没有多套一层目录 |
| 提示文件夹无法写入 | 把整套程序移到可以写入的位置再安装模型 |
| 提示找不到语音组件 | 重新完整解压发行包，保留 `libexec/` 文件夹 |

想改播报的措辞，见终端帮助里的「自定义浮层与 TTS 播报格式」。

## 从源码构建

`make desktop` 会一起构建语音程序，需要 Python 3、CMake 3.24 及以上、Ninja 和 C/C++ 编译器；Linux 另需 ALSA 开发库。只构建语音程序用 `make tts`。各平台的完整要求见：

```sh
bash scripts/build.sh --help
```

请保留模型包内的许可证。相关项目见[主项目致谢](../../README.md#致谢)。
