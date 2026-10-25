#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: bash scripts/build.sh [build|webui|overlay|overlay-qt|tts|rtmpsrt|desktop|desktop-native]

build           Build the standalone TUI (and the WebUI launcher on Windows).
desktop         Build the TUI, speech helper, and both overlay backends.
desktop-native  Build without Qt; remove Qt files owned by an earlier build.
webui           Build the Windows-only WebUI launcher.
overlay         Build the native overlay helper.
overlay-qt      Build and deploy the Qt overlay (native host only).
tts             Build the speech helper and its runtime libraries (playback and local models on GPU or CPU).
rtmpsrt         Deploy optional libsrt and its dependencies beside an existing TUI.

Environment: GOOS, GOARCH, VERSION, OUT_DIR (default: bin), BUILD_JOBS (default: 2).
Relative OUT_DIR paths are resolved from the project root.
Qt builds require CMake >= 3.24, Python 3, and the matching native Qt toolchain.
Linux overlay builds also require patchelf, pkg-config, and xkeyboard-config data.
LINUX_LICENSE_DIR optionally supplies redistribution licenses and SOURCE.txt for non-Debian builds.
RTMPSRT packaging requires SRT_LIBRARY and SRT_LICENSE_DIR (including SOURCE.txt).
The normal TUI build does not require or bundle libsrt.
Speech helper builds run on the target system and CPU (Windows x64 can also build x86).
GPU runtimes and the setup-*-qt.sh Qt SDKs come from verified prebuilt packs listed in
scripts/native-deps.json when they match the local build scripts. NATIVE_DEPS=auto (default)
builds missing or outdated packs from source, prebuilt requires the packs, and source always builds.
Source builds of the speech runtime require Python 3, CMake >= 3.24, Ninja, and a C/C++ compiler.
Windows builds use a MinGW-w64 compiler for the helper: set TTS_CC, TTS_CXX, and TTS_OBJDUMP
(CI uses llvm-mingw). Source builds of the Windows GPU runtime also need Visual Studio 2022 or 2026 C++ tools
for the target CPU; its runtime DLLs come from VCToolsRedistDir (override: TTS_MSVC_REDIST_DIR),
which must contain x64/ or arm64/Microsoft.VC*.CRT. Redistributing them requires a licensed
Visual Studio installation. TTS_MSVC_LICENSE_FILE supplies the Visual C++ runtime license offline;
otherwise it is downloaded from Microsoft.
GPU support: macOS 14+ on Apple Silicon, and Linux and Windows on x64 or ARM64.
Intel macOS and 32-bit Windows builds support CPU only.
Linux also requires the Vulkan loader, patchelf and the distribution's package license data.
TTS_VULKAN_LIBRARY optionally supplies the full path to libvulkan.so.1 for Linux packaging.
Linux packages bundle non-glibc dependencies with relative runtime paths; LD_LIBRARY_PATH is not required to run them.
TTS_LICENSE_DIR supplies the ONNX Runtime license files offline.
TTS_SHERPA_SOURCE supplies unpacked sherpa-onnx sources, and TTS_WEBGPU_PACKAGE supplies the WebGPU NuGet package.
TTS_NATIVE_DEPS_DIR supplies unpacked CMake dependencies for offline builds.
TTS_NATIVE_CMAKE_ARGS passes extra options to the native CMake build.
EOF
}

target="${1:-build}"
if (( $# > 1 )); then
  usage >&2
  exit 2
fi
case "$target" in
  -h|--help) usage; exit 0 ;;
  build|webui|overlay|overlay-qt|tts|rtmpsrt|desktop|desktop-native) ;;
  *) usage >&2; exit 2 ;;
esac
if [[ -n "${BUILD_JOBS:-}" && ! "$BUILD_JOBS" =~ ^[1-9][0-9]*$ ]]; then
  echo 'BUILD_JOBS must be a positive integer.' >&2
  exit 2
fi

# 源码定位不依赖调用目录；相对输出路径保持以项目根目录为基准。
project_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)"
cd "$project_dir"
# 部署脚本互相导入，禁止在源码目录写入字节码缓存。
export PYTHONDONTWRITEBYTECODE=1
out_dir="${OUT_DIR:-bin}"
version="${VERSION:-}"
export GOOS="${GOOS:-$(go env GOOS)}"
export GOARCH="${GOARCH:-$(go env GOARCH)}"
suffix=
overlay_cgo=1
overlay_tags=
case "$GOOS" in
  windows) suffix=.exe; overlay_cgo=0 ;;
  linux) overlay_tags=wayland ;;
esac
ldflags='-s -w -buildid='
tui_ldflags="$ldflags"
if [[ -n "$version" ]]; then
  tui_ldflags+=" -X main.version=${version#v}"
fi

require_native_qt() {
  if [[ "$GOOS" != "$(go env GOHOSTOS)" || "$GOARCH" != "$(go env GOHOSTARCH)" ]]; then
    echo 'Elegant overlay requires a matching host architecture and C++ toolchain.' >&2
    exit 2
  fi
  if [[ "$GOOS" == windows && "$GOARCH" != amd64 ]]; then
    echo 'Windows Qt builds require amd64; use desktop-native for other architectures.' >&2
    exit 2
  fi
}

# 子 shell 隔离链接搜索路径；所有 Qt 输入先构建到私有目录，部署成功后再发布。
build_qt() (
  local build_dir install_dir stage_dir native_stage_dir qt_ldflags
  mkdir -p "$out_dir/.overlay-qt-build"
  build_dir="$(cd "$out_dir/.overlay-qt-build" && pwd -P)"
  install_dir="$(cd "$out_dir" && pwd -P)/libexec"
  stage_dir="$(mktemp -d "$out_dir/.overlay-qt-stage.XXXXXX")"
  trap 'rm -rf -- "$stage_dir"' EXIT
  native_stage_dir="$(cd "$stage_dir" && pwd -P)"
  if [[ "$GOOS" == windows ]]; then
    # CGO 环境变量中的路径不会经过 MSYS 参数转换，链接器需要 Windows 路径。
    native_stage_dir="$(cygpath -m "$native_stage_dir")"
  fi
  # 清除旧 SDK、编译器与生成器的查找结果，复用目录时不混用工具链。
  cmake --fresh -S internal/overlay/qt/cpp -B "$build_dir" -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_INSTALL_PREFIX="$native_stage_dir" -DCMAKE_INSTALL_LIBDIR=.
  cmake --build "$build_dir" --config Release --parallel "${BUILD_JOBS:-2}"
  cmake --install "$build_dir" --config Release
  if [[ "$GOOS" == linux ]]; then
    # 链接和部署都需要私有依赖目录；最终包仍由部署脚本改为相对路径。
    LD_LIBRARY_PATH="$(<"$build_dir/runtime-path")${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}"
    export LD_LIBRARY_PATH
  fi
  qt_ldflags="$ldflags"
  case "$GOOS" in
    darwin) qt_ldflags+=' -extldflags=-Wl,-rpath,@loader_path' ;;
    linux) qt_ldflags+=' -extldflags=-Wl,-rpath,$ORIGIN' ;;
  esac
  CGO_ENABLED=1 CGO_LDFLAGS="${CGO_LDFLAGS:-} \"-L$native_stage_dir\"" \
    go build -tags qt -trimpath -buildvcs=false -ldflags="$qt_ldflags" \
    -o "$native_stage_dir/arcana-world-overlay-qt$suffix" ./cmd/arcana-world-overlay
  python3 scripts/deploy-qt.py "$stage_dir" "$install_dir"
)

# Linux 原生后端也必须独立收集运行库，不能借用上一轮 Qt 部署的副本。
build_native_linux() (
  local stage_dir install_dir
  if [[ "$GOOS" != "$(go env GOHOSTOS)" || "$GOARCH" != "$(go env GOHOSTARCH)" ]]; then
    echo 'Linux portable overlay requires a matching Linux host and architecture.' >&2
    exit 2
  fi
  mkdir -p "$out_dir"
  install_dir="$(cd "$out_dir" && pwd -P)/libexec"
  stage_dir="$(mktemp -d "$out_dir/.overlay-native-stage.XXXXXX")"
  trap 'rm -rf -- "$stage_dir"' EXIT
  CGO_ENABLED=1 go build -tags "$overlay_tags" -trimpath -buildvcs=false \
    -ldflags="$ldflags" -o "$stage_dir/arcana-world-overlay-native" ./cmd/arcana-world-overlay
  python3 scripts/deploy_linux.py "$stage_dir" "$install_dir"
)

# WebGPU 目标重建固定版本 C ABI；CPU-only 目标继续使用官方原生运行库。
build_tts() (
  if [[ "$GOOS" != "$(go env GOHOSTOS)" || "$GOARCH" != "$(go env GOHOSTARCH)" ]]; then
    if [[ "$GOOS" != windows || "$(go env GOHOSTOS)" != windows || "$GOARCH" != 386 ]]; then
      echo 'TTS packaging requires a matching native host/toolchain (Windows x64 may target x86).' >&2
      exit 2
    fi
  fi
  export CC="${TTS_CC:-${CC:-cc}}" CXX="${TTS_CXX:-${CXX:-c++}}"
  export OBJDUMP="${TTS_OBJDUMP:-${OBJDUMP:-objdump}}"
  if [[ "$GOOS" == windows ]]; then
    export CGO_LDFLAGS="${CGO_LDFLAGS:-} -static-libgcc"
  fi
  local stage_dir native_dir status=0
  mkdir -p "$out_dir"
  stage_dir="$(mktemp -d "$out_dir/.tts-stage.XXXXXX")"
  trap 'rm -rf -- "$stage_dir"' EXIT
  if [[ "$GOOS/$GOARCH" != darwin/amd64 && "$GOOS/$GOARCH" != windows/386 ]]; then
    python3 scripts/native-deps.py fetch tts "$GOOS" "$GOARCH" "$stage_dir" || status=$?
    if (( status == 3 )); then
      # 源码构建结果保留在工作目录，依赖工作流从这里打包。
      native_dir="$out_dir/.tts-native-$GOOS-$GOARCH"
      rm -rf -- "$native_dir/stage"
      python3 scripts/build-tts-native.py "$native_dir" "$native_dir/stage" "$GOOS" "$GOARCH"
      cp -R "$native_dir/stage/." "$stage_dir/"
    elif (( status != 0 )); then
      exit "$status"
    fi
    export TTS_NATIVE_DIR="$stage_dir"
  else
    unset TTS_NATIVE_DIR
  fi
  CGO_ENABLED=1 go build -tags tts -trimpath -buildvcs=false \
    -ldflags="$ldflags" -o "$stage_dir/arcana-world-tts$suffix" ./cmd/arcana-world-tts
  python3 scripts/deploy-tts.py "$stage_dir" "$out_dir/libexec" "$GOOS" "$GOARCH"
)

build() {
  local component="$1" cgo tags='' output flags="$ldflags" command
  if [[ "$component" == tts ]]; then
    build_tts
    return
  fi
  if [[ "$component" == overlay && "$GOOS" == linux ]]; then
    build_native_linux
    return
  fi
  case "$component" in
    build)
      cgo=0; command=arcana-world
      output="arcana-world$suffix"; flags="$tui_ldflags"
      ;;
    webui)
      cgo=0; command=arcana-world-webui
      output=arcana-world-webui.exe
      ;;
    overlay)
      cgo="$overlay_cgo"; tags="$overlay_tags"; command=arcana-world-overlay
      output="libexec/arcana-world-overlay-native$suffix"
      ;;
    *) echo "Unknown Go component: $component" >&2; exit 2 ;;
  esac
  mkdir -p "$(dirname "$out_dir/$output")"
  CGO_ENABLED="$cgo" go build -tags "$tags" -trimpath -buildvcs=false \
    -ldflags="$flags" -o "$out_dir/$output" "./cmd/$command"
  if [[ "$component" == build && "$GOOS" == windows ]]; then
    build webui
  fi
}

# 完整桌面包在构建主程序前检查 Qt 平台，避免失败后留下半套新程序。
case "$target" in
  desktop|overlay-qt) require_native_qt ;;
  webui)
    if [[ "$GOOS" != windows ]]; then
      echo 'webui: the launcher is Windows-only' >&2
      exit 2
    fi
    ;;
esac
if [[ "$target" == desktop-native && ! -f "$out_dir/libexec/.arcana-qt-files.json" &&
      ( -e "$out_dir/libexec/arcana-world-overlay-qt$suffix" || -L "$out_dir/libexec/arcana-world-overlay-qt$suffix" ) ]]; then
  echo 'Existing unmanaged Qt runtime; use a clean or new OUT_DIR for desktop-native.' >&2
  exit 2
fi

case "$target" in
  rtmpsrt)
    if [[ "$GOOS" != "$(go env GOHOSTOS)" || "$GOARCH" != "$(go env GOHOSTARCH)" ]]; then
      echo 'RTMPSRT runtime deployment requires the matching native host.' >&2
      exit 2
    fi
    python3 scripts/deploy-srt.py "$out_dir"
    ;;
  overlay-qt) build_qt ;;
  desktop|desktop-native)
    build build
    build overlay
    build tts
    if [[ "$target" == desktop ]]; then
      build_qt
    elif [[ -f "$out_dir/libexec/.arcana-qt-files.json" ]]; then
      python3 scripts/deploy-qt.py --remove "$out_dir/libexec"
    fi
    ;;
  *) build "$target" ;;
esac
