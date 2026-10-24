#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == -h || "${1:-}" == --help ]]; then
  printf '%s\n' "Usage: source ${BASH_SOURCE[0]} [--cache-info]" \
    'Build the native Qt SDK, or print its cache prefix and toolchain identity.' \
    'Environment: QT_BUILD_PREFIX, QT_BUILD_WORK, BUILD_JOBS, NATIVE_DEPS.' \
    'A matching prebuilt SDK from scripts/native-deps.json is used instead of building (NATIVE_DEPS=source disables it).' \
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

# 仅在 MSYS2 UCRT64 中构建 amd64；Qt 保持动态链接，Windows 字体由系统提供。
if [[ "${MSYSTEM:-}" != UCRT64 || -z "${MINGW_PREFIX:-}" ]]; then
  echo 'Run this script in an MSYS2 UCRT64 shell.' >&2
  exit 1
fi
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
prefix="$(cygpath -am "${QT_BUILD_PREFIX:-$HOME/.cache/arcana-qt-6.8.3-windows-minimal-v1}")"
work="$(cygpath -am "${QT_BUILD_WORK:-${RUNNER_TEMP:-/tmp}/arcana-qt-windows-minimal-v1}")"
mingw_prefix="$(cygpath -am "$MINGW_PREFIX")"
PATH="$(cygpath -u "$mingw_prefix")/bin:$PATH"
export PATH
python="$mingw_prefix/bin/python.exe"
prefix="$("$python" "$script_dir/qt-build-paths.py" canonical "$prefix")"
work="$("$python" "$script_dir/qt-build-paths.py" canonical "$work")"
"$python" "$script_dir/qt-build-paths.py" check "$prefix" "$work"
# 同一指纹绑定编译器、运行库、脚本及影响配置的环境，供本地标记和 CI 共用。
toolchain_id="$({
  set -e
  if [[ -n "${CMAKE_TOOLCHAIN_FILE:-}" ]]; then sha256sum < "$CMAKE_TOOLCHAIN_FILE"; fi
  gcc --version; g++ --version; ld --version; cmake --version; ninja --version
  # 只绑定 UCRT64 包，MSYS 的下载、压缩等工具更新不触发整套 Qt 重编译。
  pacman -Q | while read -r package version; do
    case "$package" in
      mingw-w64-ucrt-x86_64-*) printf '%s %s\n' "$package" "$version" ;;
    esac
  done | LC_ALL=C sort
  printf '%s\n' "$prefix" "$mingw_prefix" "${CFLAGS:-}" "${CXXFLAGS:-}" "${CPPFLAGS:-}" "${LDFLAGS:-}"
  printf '%s\n' "${CMAKE_TOOLCHAIN_FILE:-}"
  sha256sum < "${BASH_SOURCE[0]}"; sha256sum < "$script_dir/qt-build-paths.py"
} | sha256sum | cut -d ' ' -f 1)"
# 依赖清单中有与本脚本一致的预编译 SDK 时直接使用；缓存键随包内容变化。
status=0
pack="$("$python" "$script_dir/native-deps.py" resolve qt windows amd64)" || status=$?
if (( status == 0 )); then
  toolchain_id="prebuilt-$pack"
elif (( status != 3 )); then
  exit "$status"
fi
if [[ "${1:-}" == --cache-info ]]; then
  printf 'prefix=%s\ntoolchain-id=%s\n' "$prefix" "$toolchain_id"
  return 0 2>/dev/null || exit 0
fi
if [[ "$toolchain_id" == prebuilt-* && "$(cat "$prefix/.complete" 2>/dev/null)" != "$toolchain_id" ]]; then
  "$python" "$script_dir/qt-build-paths.py" reset "$prefix" "$work"
  "$python" "$script_dir/native-deps.py" fetch qt windows amd64 "$prefix"
  printf '%s\n' "$toolchain_id" > "$prefix/.complete"
fi
if [[ ! -f "$prefix/.complete" || "$(cat "$prefix/.complete")" != "$toolchain_id" ]]; then
  # 失败后从已确认所有权的干净目录重建；不能保留被新配置裁剪掉的 DLL。
  "$python" "$script_dir/qt-build-paths.py" reset "$prefix" "$work"
  mkdir -p "$prefix/share/licenses/qtbase"
  curl --fail --location --retry 3 \
    https://download.qt.io/archive/qt/6.8/6.8.3/submodules/qtbase-everywhere-src-6.8.3.tar.xz \
    -o "$work/qtbase.tar.xz"
  printf '%s  %s\n' 56001b905601bb9023d399f3ba780d7fa940f3e4861e496a7c490331f49e0b80 "$work/qtbase.tar.xz" | sha256sum --check --strict
  tar --force-local -xf "$work/qtbase.tar.xz" -C "$work"
  source_dir="$work/qtbase-everywhere-src-6.8.3"
  # 保留字体整形、彩色字形和光栅 QWidget；使用内置第三方库减少外部 DLL。
  # Qt 6.8 的 DirectWrite 字体加载依赖 Direct2D，不能随 GPU 后端一起禁用。
  cmake -S "$source_dir" -B "$work/qtbase-build" -GNinja \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" \
    -DCMAKE_C_COMPILER="$mingw_prefix/bin/gcc.exe" -DCMAKE_CXX_COMPILER="$mingw_prefix/bin/g++.exe" \
    -DQT_BUILD_TESTS=OFF -DQT_BUILD_EXAMPLES=OFF -DBUILD_SHARED_LIBS=ON \
    -DFEATURE_optimize_size=ON -DFEATURE_gui=ON -DFEATURE_widgets=ON \
    -DFEATURE_icu=OFF -DFEATURE_glib=OFF -DFEATURE_dbus=OFF -DFEATURE_zstd=OFF \
    -DFEATURE_network=OFF -DFEATURE_sql=OFF -DFEATURE_xml=OFF \
    -DFEATURE_concurrent=OFF -DFEATURE_printsupport=OFF -DFEATURE_testlib=OFF \
    -DFEATURE_itemviews=OFF -DFEATURE_graphicsview=OFF -DFEATURE_mainwindow=OFF \
    -DFEATURE_dialog=OFF -DFEATURE_textedit=OFF -DFEATURE_calendarwidget=OFF \
    -DFEATURE_filesystemmodel=OFF -DFEATURE_pdf=OFF -DFEATURE_style_stylesheet=OFF \
    -DFEATURE_style_windowsvista=OFF -DFEATURE_style_windows11=OFF \
    -DFEATURE_textmarkdownreader=OFF -DFEATURE_textmarkdownwriter=OFF -DFEATURE_textodfwriter=OFF \
    -DINPUT_opengl=no -DFEATURE_opengl=OFF -DFEATURE_egl=OFF -DFEATURE_vulkan=OFF \
    -DFEATURE_direct2d=ON -DFEATURE_directwrite=ON -DFEATURE_directwrite3=ON \
    -DFEATURE_freetype=ON -DFEATURE_harfbuzz=ON -DFEATURE_pcre2=ON \
    -DFEATURE_system_zlib=OFF -DFEATURE_system_pcre2=OFF \
    -DFEATURE_system_freetype=OFF -DFEATURE_system_harfbuzz=OFF \
    -DFEATURE_system_png=OFF -DFEATURE_system_jpeg=OFF \
    -DFEATURE_system_doubleconversion=OFF -DFEATURE_system_libb2=OFF \
    -DINPUT_openssl=no
  cmake --build "$work/qtbase-build" --parallel "${BUILD_JOBS:-2}"
  cmake --install "$work/qtbase-build" --strip
  cp -R "$source_dir/LICENSES" "$prefix/share/licenses/qtbase/"
  # 除通用许可文本外，保留每个内嵌组件的版权归属与其引用的专用许可文件。
  "$mingw_prefix/bin/python.exe" - "$source_dir" "$prefix/share/licenses/qtbase" <<'PY'
import json
import pathlib
import shutil
import sys

source = pathlib.Path(sys.argv[1]).resolve()
destination = pathlib.Path(sys.argv[2])
# 不收集示例、测试、构建工具和其他平台的声明。
runtime_roots = [source / name for name in (
    "src/corelib", "src/gui", "src/widgets", "src/plugins/platforms/windows", "src/3rdparty",
)]
excluded = {
    "src/corelib/kernel", "src/gui/opengl", "src/gui/vulkan", "src/gui/text",
    *(f"src/3rdparty/{name}" for name in (
        "android", "gradle", "forkfd", "pixman", "sqlite", "libpsl", "wasm", "xcb",
        "VulkanMemoryAllocator", "md4c",
    )),
}
files = set()
for root in runtime_roots:
    files.update(manifest for manifest in root.rglob("qt_attribution.json")
                 if manifest.parent.relative_to(source).as_posix() not in excluded)
for manifest in sorted(files):
    # Qt 上游声明的 Copyright 字段包含未转义换行。
    entries = json.loads(manifest.read_text(encoding="utf-8"), strict=False)
    if isinstance(entries, dict):
        entries = [entries]
    for entry in entries:
        license_files = entry.get("LicenseFile", [])
        if isinstance(license_files, str):
            license_files = [license_files]
        files.update((manifest.parent / filename).resolve() for filename in license_files)
    files.update(notice for notice in manifest.parent.rglob("*")
                 if notice.is_file() and notice.name.lower().startswith(
                     ("copyright", "copying", "license", "notice")))
for path in sorted(files):
    target = destination / path.relative_to(source)
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copy2(path, target)
PY
  printf '%s\n' "$toolchain_id" > "$prefix/.complete"
fi
export CMAKE_PREFIX_PATH="$prefix"
export QTPATHS="$prefix/bin/qtpaths.exe"
export QT_LICENSE_DIR="$prefix/share/licenses"
export MINGW_PREFIX="$mingw_prefix"
export CMAKE_GENERATOR=Ninja CC=gcc CXX=g++
PATH="$(cygpath -u "$prefix")/bin:$(cygpath -u "$mingw_prefix")/bin:$PATH"
export PATH
if [[ -n "${GITHUB_ENV:-}" ]]; then
  {
    echo "CMAKE_PREFIX_PATH=$CMAKE_PREFIX_PATH"
    echo "QTPATHS=$QTPATHS"
    echo "QT_LICENSE_DIR=$QT_LICENSE_DIR"
    echo "MINGW_PREFIX=$MINGW_PREFIX"
    echo 'CMAKE_GENERATOR=Ninja'
    echo 'CC=gcc'
    echo 'CXX=g++'
  } >> "$GITHUB_ENV"
  printf '%s\n' "$prefix/bin" "$mingw_prefix/bin" >> "$GITHUB_PATH"
fi
