#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == -h || "${1:-}" == --help ]]; then
  printf '%s\n' "Usage: source ${BASH_SOURCE[0]} [--cache-info]" \
    'Build the native Qt SDK, or print its cache prefix and toolchain identity.' \
    'Environment: QT_BUILD_PREFIX, QT_BUILD_WORK, BUILD_JOBS, MACOSX_DEPLOYMENT_TARGET.' \
    'Install and work directories must be empty or owned by this script.'
  return 0 2>/dev/null || exit 0
fi
if (( $# > 1 )) || [[ -n "${1:-}" && "$1" != --cache-info ]]; then
  echo "Unknown arguments; see ${BASH_SOURCE[0]} --help." >&2
  exit 2
fi
if [[ -n "${BUILD_JOBS:-}" && ! "$BUILD_JOBS" =~ ^[1-9][0-9]*$ ]]; then
  echo 'BUILD_JOBS must be a positive integer.' >&2
  exit 2
fi

# QCocoaScreen 原生屏幕映射需要 Qt 6.11，不能使用其他平台的 6.8.3。
if [[ "$(uname -s)" != Darwin ]]; then
  echo 'Run this script on macOS with Xcode command line tools.' >&2
  exit 1
fi
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
prefix="${QT_BUILD_PREFIX:-$HOME/.cache/arcana-qt-6.11.0-macos-minimal-v1}"
work="${QT_BUILD_WORK:-${RUNNER_TEMP:-/tmp}/arcana-qt-macos-minimal-v1}"
prefix="$(python3 "$script_dir/qt-build-paths.py" canonical "$prefix")"
work="$(python3 "$script_dir/qt-build-paths.py" canonical "$work")"
python3 "$script_dir/qt-build-paths.py" check "$prefix" "$work"
arch="$(uname -m)"
sdk="$(xcrun --sdk macosx --show-sdk-path)"
export MACOSX_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-13.0}"
# 同一指纹同时用于 .complete 和 CI 缓存，包含实际前缀及部署目标。
toolchain_id="$({
  set -e
  if [[ -n "${CMAKE_TOOLCHAIN_FILE:-}" ]]; then shasum -a 256 < "$CMAKE_TOOLCHAIN_FILE"; fi
  xcrun clang --version; xcodebuild -version; xcrun --sdk macosx --show-sdk-version
  cmake --version; ninja --version
  printf '%s\n' "$sdk" "$arch" "$prefix" "$MACOSX_DEPLOYMENT_TARGET" "${CFLAGS:-}" "${CXXFLAGS:-}" "${CPPFLAGS:-}" "${LDFLAGS:-}"
  printf '%s\n' "${CMAKE_TOOLCHAIN_FILE:-}"
  shasum -a 256 < "${BASH_SOURCE[0]}"; shasum -a 256 < "$script_dir/qt-build-paths.py"
} | shasum -a 256 | cut -d ' ' -f 1)"
if [[ "${1:-}" == --cache-info ]]; then
  printf 'prefix=%s\ntoolchain-id=%s\n' "$prefix" "$toolchain_id"
  return 0 2>/dev/null || exit 0
fi
if [[ ! -f "$prefix/.complete" || "$(cat "$prefix/.complete")" != "$toolchain_id" ]]; then
  python3 "$script_dir/qt-build-paths.py" reset "$prefix" "$work"
  curl --fail --location --retry 3 \
    https://download.qt.io/archive/qt/6.11/6.11.0/submodules/qtbase-everywhere-src-6.11.0.tar.xz \
    -o "$work/qtbase.tar.xz"
  printf '%s  %s\n' 231ad85979864d914dc9568a1b71c91d6cf20d7b2021d059103bf0eb51cb755e "$work/qtbase.tar.xz" | shasum -a 256 --check
  tar -xf "$work/qtbase.tar.xz" -C "$work"
  source_dir="$work/qtbase-everywhere-src-6.11.0"
  # 普通动态库避免打包 framework 开发文件；Cocoa/CoreText 保留系统字体与彩色 emoji。
  # 保留 HarfBuzz、emoji 分段和 PNG（Cocoa 内嵌光标），不关闭文字整形。
  # 浮层不使用工具提示；Qt 6.11 的该实现仍无条件引用样式表字段，须一并裁剪。
  cmake -S "$source_dir" -B "$work/qtbase-build" -GNinja \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" \
    -DCMAKE_C_COMPILER="$(xcrun -f clang)" -DCMAKE_CXX_COMPILER="$(xcrun -f clang++)" \
    -DCMAKE_OSX_SYSROOT="$sdk" -DCMAKE_OSX_ARCHITECTURES="$arch" \
    -DCMAKE_OSX_DEPLOYMENT_TARGET="$MACOSX_DEPLOYMENT_TARGET" \
    -DQT_BUILD_TESTS=OFF -DQT_BUILD_EXAMPLES=OFF -DBUILD_SHARED_LIBS=ON \
    -DFEATURE_framework=OFF -DFEATURE_optimize_size=ON \
    -DFEATURE_gui=ON -DFEATURE_widgets=ON \
    -DFEATURE_icu=OFF -DFEATURE_glib=OFF -DFEATURE_dbus=OFF -DFEATURE_zstd=OFF \
    -DFEATURE_network=OFF -DFEATURE_sql=OFF -DFEATURE_xml=OFF \
    -DFEATURE_concurrent=OFF -DFEATURE_printsupport=OFF -DFEATURE_testlib=OFF \
    -DFEATURE_itemviews=OFF -DFEATURE_graphicsview=OFF -DFEATURE_mainwindow=OFF \
    -DFEATURE_dialog=OFF -DFEATURE_textedit=OFF -DFEATURE_calendarwidget=OFF \
    -DFEATURE_filesystemmodel=OFF -DFEATURE_pdf=OFF -DFEATURE_style_stylesheet=OFF \
    -DFEATURE_tooltip=OFF \
    -DFEATURE_textmarkdownreader=OFF -DFEATURE_textmarkdownwriter=OFF -DFEATURE_textodfwriter=OFF \
    -DFEATURE_opengl=OFF -DFEATURE_egl=OFF -DFEATURE_vulkan=OFF \
    -DFEATURE_freetype=OFF -DFEATURE_harfbuzz=ON -DFEATURE_emojisegmenter=ON \
    -DFEATURE_png=ON -DFEATURE_pcre2=ON \
    -DFEATURE_system_zlib=OFF -DFEATURE_system_pcre2=OFF \
    -DFEATURE_system_harfbuzz=OFF -DFEATURE_system_png=OFF -DFEATURE_system_jpeg=OFF \
    -DFEATURE_system_doubleconversion=OFF -DFEATURE_system_libb2=OFF
  cmake --build "$work/qtbase-build" --parallel "${BUILD_JOBS:-3}"
  cmake --install "$work/qtbase-build" --strip
  mkdir -p "$prefix/share/licenses/qtbase"
  cp -R "$source_dir/LICENSES" "$prefix/share/licenses/qtbase/"
  # 保留运行时代码与内嵌组件的原始版权声明及其引用的许可文件。
  python3 - "$source_dir" "$prefix/share/licenses/qtbase" <<'PY'
import json
from pathlib import Path
import shutil
import sys
source = Path(sys.argv[1]).resolve()
destination = Path(sys.argv[2])
files = set()
for name in ('src/corelib', 'src/gui', 'src/widgets', 'src/plugins/platforms/cocoa', 'src/3rdparty'):
    files.update((source / name).rglob('qt_attribution.json'))
for manifest in sorted(files):
    entries = json.loads(manifest.read_text(encoding='utf-8'), strict=False)
    if isinstance(entries, dict):
        entries = [entries]
    for entry in entries:
        names = entry.get('LicenseFile', [])
        if isinstance(names, str):
            names = [names]
        files.update((manifest.parent / name).resolve() for name in names)
    files.update(path for path in manifest.parent.rglob('*')
                 if path.is_file() and path.name.lower().startswith(('copyright', 'copying', 'license', 'notice')))
for path in sorted(files):
    target = destination / path.relative_to(source)
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(path, target)
PY
  printf '%s\n' "$toolchain_id" > "$prefix/.complete"
fi
export CMAKE_PREFIX_PATH="$prefix"
export QTPATHS="$prefix/bin/qtpaths"
export QT_LICENSE_DIR="$prefix/share/licenses"
export CMAKE_GENERATOR=Ninja CC=clang CXX=clang++
export PATH="$prefix/bin:$PATH"
if [[ -n "${GITHUB_ENV:-}" ]]; then
  {
    echo "CMAKE_PREFIX_PATH=$CMAKE_PREFIX_PATH"
    echo "QTPATHS=$QTPATHS"
    echo "QT_LICENSE_DIR=$QT_LICENSE_DIR"
    echo "MACOSX_DEPLOYMENT_TARGET=$MACOSX_DEPLOYMENT_TARGET"
    echo 'CMAKE_GENERATOR=Ninja'
    echo 'CC=clang'
    echo 'CXX=clang++'
  } >> "$GITHUB_ENV"
  printf '%s\n' "$prefix/bin" >> "$GITHUB_PATH"
fi
