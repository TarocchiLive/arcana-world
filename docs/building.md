# 从源码构建

这份指南面向自行编译的用户和贡献者。普通使用请直接下载发行包；快速上手见 [README](../README.md#从源码构建)。

所有命令都在**项目根目录**执行。

## 选择构建目标

| 命令 | 产物 | 说明 |
| --- | --- | --- |
| `make build` | `bin/arcana-world`（Windows 为 `bin/arcana-world.exe`，同时生成 `bin/arcana-world-webui.exe`） | 只构建终端主程序，不依赖 CGO，不需要图形或语音依赖 |
| `make overlay` | `bin/libexec/arcana-world-overlay-native` | 原生浮层 helper |
| `make overlay-qt` | Linux、Windows：`bin/libexec/arcana-world-overlay-qt`；macOS：`bin/libexec/Arcana World Elegant Overlay.app/Contents/MacOS/arcana-world-overlay-qt`；另含 Qt 运行库 | 精美浮层 helper，只能在目标系统本机构建 |
| `make tts` | `bin/libexec/arcana-world-tts` 及语音运行库 | 语音 helper |
| `make desktop` | 主程序加上述三个 helper | 完整桌面包 |
| `make desktop-native` | 主程序、原生浮层、语音 helper | 不含 Qt；会移除此前构建留下的 Qt 文件 |
| `make webui` | `bin/arcana-world-webui.exe` | 仅 Windows 的 WebUI 启动器 |
| `make run` | 无 | 执行 `go run ./cmd/arcana-world` |
| `make check` | 无 | 执行 `go test -race ./...` 和 `go vet ./...` |
| `make clean` | 无 | 见[清理](#清理) |

构建目标由 `scripts/build.sh` 执行，`make` 只是传入变量：

| 变量 | 默认值 | 作用 |
| --- | --- | --- |
| `GOOS`、`GOARCH` | `go env` 的值 | 目标系统与架构 |
| `VERSION` | 空 | 非空时写入主程序版本号（去掉前导 `v`） |
| `OUT_DIR` | `bin` | 输出目录，相对路径以项目根目录为基准 |
| `BUILD_JOBS` | `2` | Qt 与语音原生库的编译并发数，必须是正整数 |

```sh
make desktop BUILD_JOBS=8
make build GOOS=windows GOARCH=amd64 VERSION=v0.4.2
```

`overlay-qt`、`desktop` 和 `tts` 需要在与目标一致的系统和架构上构建，不支持交叉编译（Windows x64 上的 `tts` 可以目标 x86）。Windows 的 Qt 构建只支持 amd64，其他架构使用 `desktop-native`。`overlay` 在 Linux 上同样要求本机架构，并会收集运行库。

完整参数和各平台工具链要求见 `bash scripts/build.sh --help`。该脚本还支持 `rtmpsrt`，见 [RTMPSRT 运行库](#rtmpsrt-运行库)。

## 依赖

### Go

版本以 `go.mod` 的 `go` 行为准（当前为 1.26.8）。

### 只要终端功能

`make build` 只需要 Go，不需要 C 编译器、Qt、Python 或 CMake。

### 浮层

原生浮层和精美浮层的平台依赖、Qt SDK 准备方式见 [Arcana World Overlay：构建与启动](../cmd/arcana-world-overlay/README.md#构建与启动) 和[精美浮层构建与运行库](../cmd/arcana-world-overlay/README.md#精美浮层构建与运行库)。Linux 上构建浮层还需要 `patchelf`、`pkg-config` 和 xkeyboard-config 数据。

Linux 的 Qt 与 LayerShellQt 可由 `scripts/setup-linux-qt.sh` 准备，macOS、Windows 对应 `scripts/setup-macos-qt.sh`、`scripts/setup-windows-qt.sh`；各脚本的 `--help` 列出可配置项。

### 语音

`make tts`、`make desktop` 和 `make desktop-native` 会构建语音 helper：

- Python 3、C/C++ 编译器；
- 从源码构建语音运行库时，另需 CMake >= 3.24 和 Ninja；
- Linux 另需 ALSA 开发库和 `pkg-config`（Debian/Ubuntu：`libasound2-dev pkg-config`），以及 Vulkan loader、`patchelf` 和发行版软件包的许可数据；
- Windows 使用 MinGW-w64 编译 helper，通过 `TTS_CC`、`TTS_CXX`、`TTS_OBJDUMP` 指定；从源码构建 Windows GPU 运行库还需要 Visual Studio 2022 或 2026 的 C++ 工具。细节见 `bash scripts/build.sh --help`。

语音 helper 的 GPU 运行库支持 macOS 14+ 的 Apple Silicon，以及 Linux、Windows 的 x64 和 ARM64；Intel macOS 和 32 位 Windows 只支持 CPU。

Linux 部署脚本按 SONAME 打包 `libvulkan.so.1`；若构建环境无法自动定位，可通过 `TTS_VULKAN_LIBRARY=/path/to/libvulkan.so.1` 指定。显卡驱动由运行机器提供，不随包分发。NixOS 更新显卡驱动后，旧构建产物的 glibc 可能无法满足新 Mesa/LLVM 的要求；遇到 `GLIBC_x.y not found` 时，在当前 `nix develop` 环境重新构建语音 helper。

### RTMPSRT 运行库

默认构建不包含 libsrt。需要把它打包进发行目录时，在已有主程序旁执行：

```sh
SRT_LIBRARY=/path/to/libsrt.so SRT_LICENSE_DIR=/path/to/licenses bash scripts/build.sh rtmpsrt
```

- `SRT_LIBRARY`：libsrt 动态库文件（Windows 为 `srt.dll`，macOS 为 `libsrt.dylib`）。
- `SRT_LICENSE_DIR`：libsrt 及其依赖的许可目录，必须包含 `SOURCE.txt`。

`scripts/deploy-srt.py` 会把库和它的依赖复制到 `OUT_DIR/lib`，写入 `licenses/srt`，并在 macOS、Linux 上改写为相对加载路径。主程序从自身所在目录的 `lib` 加载 libsrt。该目标要求本机系统和架构。

## 语音原生库

`make tts` 先尝试下载预编译包，没有可用的预编译包时自动改为从源码构建。`NATIVE_DEPS` 控制这一选择：

| 取值 | 行为 |
| --- | --- |
| `auto`（默认） | 预编译包存在且与本地构建脚本匹配时下载；否则从源码构建 |
| `prebuilt` | 必须使用预编译包，找不到或已过期则报错 |
| `source` | 始终从源码构建 |

预编译包列在 `scripts/native-deps.json`，该文件不存在时 `auto` 直接从源码构建。`python3 scripts/native-deps.py status` 列出清单中的包以及它们是否与本地脚本匹配。`scripts/build-tts-native.py`、`scripts/tts-native/sources.json` 或 `scripts/tts-native/arcana-webgpu.h` 改动后，对应预编译包即视为过期。

`scripts/setup-*-qt.sh` 准备 Qt SDK 时使用同一份清单和 `NATIVE_DEPS`；对应平台的 setup 脚本或 `scripts/qt-build-paths.py` 改动后，该平台的 Qt 包过期。预编译包由 `.github/workflows/native-deps.yml` 在推送 `native-deps-*` 标签时从源码构建并发布，取回附件中的 `native-deps.json` 提交为 `scripts/native-deps.json` 后生效；发版工作流要求所有包都是最新的，步骤见 [arcana-release](../.agents/skills/arcana-release/SKILL.md)。

Intel macOS（`darwin/amd64`）和 32 位 Windows（`windows/386`）不使用上述流程，直接使用官方 sherpa-onnx 的 CPU 运行库。

### 下载来源与校验

- 预编译包：地址取自 `scripts/native-deps.json`，按其中记录的 SHA256 校验，缓存在 `$XDG_CACHE_HOME/arcana-native-deps`（默认 `~/.cache/arcana-native-deps`），再次构建直接复用。下载时每 25% 输出一次进度。
- 源码构建：sherpa-onnx 源码包与 WebGPU NuGet 包的地址和 SHA256 记录在 `scripts/tts-native/sources.json`，下载后逐一校验，不符即报错；CMake 拉取的依赖也要与该清单中的 SHA256 一致。

源码构建下载 sherpa-onnx 源码和 WebGPU 包时没有进度输出，网络慢时看起来像卡住，请耐心等待。网络受限时，这两处下载使用 Python 的 `urllib`，会读取 `https_proxy`、`http_proxy` 环境变量：

```sh
https_proxy=http://127.0.0.1:7890 http_proxy=http://127.0.0.1:7890 make tts
```

### 构建缓存

源码构建的工作目录是 `OUT_DIR/.tts-native-<goos>-<goarch>`（默认 `bin/.tts-native-linux-amd64` 这类路径），其中：

- `sherpa`、`webgpu`：已下载并校验过的源码包和 NuGet 包，再次构建直接复用；
- `source`、`stage`：每次构建重新生成；
- `build`：CMake 构建目录。

`make clean` 会删除整个工作目录；想保留已下载的包，请先复制 `sherpa` 和 `webgpu`。

### 环境变量

| 变量 | 作用 |
| --- | --- |
| `TTS_SHERPA_SOURCE` | 已解包的 sherpa-onnx 源码目录，跳过下载。内容会复制到私有目录再打补丁，原目录保持不变；需与 `sources.json` 锁定的版本一致，否则补丁校验失败 |
| `TTS_WEBGPU_PACKAGE` | 本地 WebGPU NuGet 包（`.nupkg`）路径，仍按 `sources.json` 校验 SHA256 |
| `TTS_NATIVE_DEPS_DIR` | 离线依赖目录。每个子目录是一个已解包的 CMake 依赖，目录名为 FetchContent 名称（`sources.json` 中的 `cmake_name`：`kaldifst`、`openfst`、`kissfft`、`eigen`、`kaldi_native_fbank`、`kaldi_decoder`、`simple-sentencepiece`、`json`、`espeak_ng`、`piper_phonemize`、`onnxruntime`）。设置后 CMake 不再联网，也不再核对 FetchContent 的 SHA256 |
| `TTS_NATIVE_CMAKE_ARGS` | 追加到原生库 CMake 配置命令的参数，按 shell 规则拆分 |
| `TTS_NATIVE_CMAKE_GENERATOR` | CMake 生成器；默认 `Ninja`，Windows 按已加载的 Visual Studio 环境选择 2022 或 2026 |
| `BUILD_JOBS` | 编译并发数，默认 2 |
| `TTS_LICENSE_DIR` | 离线提供 ONNX Runtime 许可文件（`onnxruntime-LICENSE`、`onnxruntime-ThirdPartyNotices.txt`），内容仍按 SHA256 校验 |

`TTS_NATIVE_DEPS_DIR` 的目录布局与 `flake.nix` 中 `arcana-world-tts` 的做法相同，可作参考。

## Nix

`flake.nix` 支持 `x86_64-linux`、`aarch64-linux`、`x86_64-darwin`、`aarch64-darwin`。

### 构建包

```sh
nix build
./result/bin/arcana-world
```

`packages` 提供：

| 包 | 内容 |
| --- | --- |
| `default`、`arcana-world-desktop` | 主程序、原生浮层、精美浮层和语音 helper |
| `arcana-world-desktop-native` | 主程序、原生浮层和语音 helper，不含 Qt |
| `arcana-world` | 只含终端主程序 |
| `arcana-world-overlay-native` | 原生浮层 helper |
| `arcana-world-overlay-qt` | 精美浮层 helper |
| `arcana-world-tts` | 语音 helper |

使用 `nix build .#<包名>` 构建指定的包。完整包的 helper 位于 `result/bin/libexec`。macOS 上的 `arcana-world-overlay-qt` 与完整包要求 nixpkgs 的 Qt >= 6.11，版本不足时报错，可改用 `arcana-world-desktop-native`。

`nix run`（默认应用）和 `nix run .#arcana-world-desktop` 运行完整包。

### 开发环境

`nix develop` 进入 `default` 开发环境，包含 Go、CMake、Python 3、`pkg-config`、Qt 6（Linux 另含 `patchelf`、Wayland 与 LayerShellQt、xkeyboard-config、fontconfig），并设置 `CGO_ENABLED=1` 和 `NATIVE_DEPS=source`：Nix 环境不下载预编译包，Qt 使用 nixpkgs 提供的版本，语音运行库从源码构建。在其中使用 `make`：

```sh
nix develop -c make overlay-qt
nix develop -c make desktop
```

可用的开发环境：

| 名称 | 用途 |
| --- | --- |
| `default` | 完整桌面构建（终端、两种浮层、语音） |
| `arcana-world` | 仅 Go，`CGO_ENABLED=0`，用于终端主程序 |
| `arcana-world-overlay-native` | 原生浮层（Linux 含 Wayland、Pango、Cairo 开发库） |
| `arcana-world-tts` | 语音 helper（CMake、Ninja、Python 3，Linux 含 ALSA） |

使用 `nix develop .#<名称>` 进入指定环境。

## 清理

```sh
make clean
```

`make clean` 删除：

- `$(OUT_DIR)/arcana-world`、`arcana-world.exe`、`arcana-world-webui.exe`；
- `$(OUT_DIR)/libexec`，即全部 helper、Qt 与语音运行库；
- `$(OUT_DIR)/.overlay-qt-build`；
- `$(OUT_DIR)/.tts-native-*`，包括已下载的源码包。

`$(OUT_DIR)/lib`（RTMPSRT 运行库）不在清理范围内。

## 运行构建产物

```sh
./bin/arcana-world
```

Windows 为 `bin/arcana-world.exe`。主程序从自身所在目录的 `libexec/` 查找 helper（Windows 加 `.exe`），macOS 的 Qt helper 直接使用应用包内路径：

| 文件 | 来源 |
| --- | --- |
| `bin/libexec/arcana-world-overlay-native` | `make overlay`、`desktop`、`desktop-native` |
| `bin/libexec/arcana-world-overlay-qt`（Linux、Windows） | `make overlay-qt`、`desktop` |
| `bin/libexec/Arcana World Elegant Overlay.app/Contents/MacOS/arcana-world-overlay-qt`（macOS） | `make overlay-qt`、`desktop` |
| `bin/libexec/arcana-world-tts` | `make tts`、`desktop`、`desktop-native` |

`libexec` 里的运行库和 `.arcana-*-files.json` 清单由部署脚本维护，请保持目录完整，搬迁时整体移动。重复构建会清理已移除的依赖；清单缺失的旧输出目录请换一个新的 `OUT_DIR`。

macOS 的 Qt 应用包内，Qt 运行库在 `Contents/Frameworks`，平台插件在 `Contents/PlugIns/platforms`，`qt.conf` 在 `Contents/Resources`。发行包附带双语 `看我.txt`；确认来源可信后，在解压后的包根目录执行 `xattr -dr com.apple.quarantine .`，再执行 `./arcana-world`。每次升级完整替换包后重复此操作；只允许主程序运行，语音 helper 仍可能被系统拦截。

Linux 语音 helper 在初始化 GPU 时，通过 Vulkan Loader 的 [`VK_ADD_DRIVER_FILES`](https://github.com/KhronosGroup/Vulkan-Loader/blob/main/docs/LoaderDriverInterface.md#additional-driver-discovery) 追加存在的 `/run/opengl-driver/share/vulkan/icd.d`，使便携包也能发现 NixOS 系统驱动。标准驱动搜索和过滤规则保持不变；已有追加路径保留且优先，显式设置 `VK_DRIVER_FILES` 或 `VK_ICD_FILENAMES` 时完全沿用用户配置。设置仅作用于语音 helper，不改变主程序环境。显卡驱动仍由系统提供。

```sh
./bin/arcana-world -overlay
./bin/arcana-world -doctor
```

`-overlay` 启用浮层，`-doctor` 检查配置、凭据存储及可选辅助程序。浮层的启用与调整见 [Arcana World Overlay](../cmd/arcana-world-overlay/README.md#随-arcana-world-启动)，语音见[语音播报说明](../cmd/arcana-world-tts/README.md)。

### 语音冒烟验证

`internal/tts/real_synthesis_test.go`（构建标签 `ttssmoke`）走真实链路：Edge 在线合成，或安装固定模型后由已构建的语音 helper 合成，并检查返回的音频时长与音量。测试程序须放在应用目录，与 `libexec/` 同级；模型安装在该目录的 `models/tts/`，已有的压缩包会直接校验安装。

```sh
OUT_DIR=.local/tts-smoke bash scripts/build.sh tts
go test -c -tags ttssmoke -o .local/tts-smoke/tts-smoke.test ./internal/tts
ARCANA_TTS_SMOKE=edge,melo,kokoro,piper ARCANA_TTS_SMOKE_BACKEND=cpu \
  ./.local/tts-smoke/tts-smoke.test -test.run '^TestRealSynthesis$' -test.v -test.timeout 45m
```

`ARCANA_TTS_SMOKE_BACKEND` 可选 `cpu` 或 `webgpu`。`.github/workflows/tts-smoke.yml` 在托管 runner 上按 CI 的七种平台组合验证 CPU；托管 runner 没有可用 GPU，WebGPU 只能在本机验证。

## 常见问题

### `‘-Wformat-security’ ignored without ‘-Wformat’`

在 Nix 环境构建语音原生库时，sherpa-onnx 编译 espeak-ng 带有 `-Wno-format`，与 Nix 默认的 `-Werror=format-security` 冲突。`flake.nix` 的 `default` 和 `arcana-world-tts` 开发环境已设置 `hardeningDisable = [ "format" ]`。如果 shell 是在此设置加入之前进入的，退出后重新执行 `nix develop`。

### 语音库下载很慢或像卡住

源码构建下载 sherpa-onnx 和 WebGPU 包时没有进度输出。可设置 `https_proxy`、`http_proxy` 走代理，或预先下载后通过 `TTS_SHERPA_SOURCE`、`TTS_WEBGPU_PACKAGE`、`TTS_NATIVE_DEPS_DIR` 离线构建，见[环境变量](#环境变量)。下载过的包保存在 `bin/.tts-native-<goos>-<goarch>`，重试时复用。

### `WARNING: Local Nix build: complete redistribution license notices are not installed`

Linux 上运行库全部来自 `/nix/store` 时，部署脚本无法收集发行版的许可声明，只写入一份说明 `SOURCE.txt` 并给出此警告。构建出的程序可以本地使用；需要再分发时，准备一个包含各依赖许可文件和 `SOURCE.txt` 的目录，通过 `LINUX_LICENSE_DIR` 提供后重新构建：

```sh
LINUX_LICENSE_DIR=/path/to/licenses make desktop
```

### 其他报错

| 提示 | 处理 |
| --- | --- |
| `BUILD_JOBS must be a positive integer.` | `BUILD_JOBS` 设为正整数 |
| `Elegant overlay requires a matching host architecture and C++ toolchain.` | 在目标系统和架构上构建，或改用 `desktop-native` |
| `Windows Qt builds require amd64; use desktop-native for other architectures.` | 改用 `make desktop-native` |
| `webui: the launcher is Windows-only` | `webui` 目标仅在 `GOOS=windows` 时可用 |
| `Existing unmanaged Qt runtime; use a clean or new OUT_DIR for desktop-native.` | 输出目录里的 Qt 文件不是这套脚本部署的，使用新的 `OUT_DIR` |
| `Existing unmanaged runtime file: ...; use a clean/new output directory` | 同上 |
| `Prebuilt ... predates the local build scripts`、`No prebuilt ... in scripts/native-deps.json` | `NATIVE_DEPS=auto` 会自动改为源码构建；`prebuilt` 模式下改用 `auto` 或 `source` |
| `NATIVE_DEPS must be auto, prebuilt, or source` | 修正 `NATIVE_DEPS` 的取值 |
| `SHA256 mismatch: ...` | 下载内容与清单不符；删除缓存文件后重试，离线提供的文件需使用清单锁定的版本 |
| `Pinned sherpa source no longer matches patch: ...` | `TTS_SHERPA_SOURCE` 与 `sources.json` 锁定的版本不一致 |
| `Linux portable runtime deployment requires <tool>` | 安装 `patchelf` 等提示的工具 |
| `Linux dependency licenses require dpkg-query or LINUX_LICENSE_DIR with SOURCE.txt` | 非 Debian 系统通过 `LINUX_LICENSE_DIR` 提供许可目录和 `SOURCE.txt` |
| `Set SRT_LIBRARY ...`、`Set SRT_LICENSE_DIR ...` | 按[RTMPSRT 运行库](#rtmpsrt-运行库)设置两个变量 |
| `Unsupported Visual Studio ...; set TTS_NATIVE_CMAKE_GENERATOR` | 在 Visual Studio 2022 或 2026 开发者环境中构建，或手动指定生成器 |
