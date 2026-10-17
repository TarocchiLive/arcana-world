#!/usr/bin/env bash
set -euo pipefail

# 仅在 MSYS2 UCRT64 中构建 amd64；Qt 保持动态链接，Windows 字体由系统提供。
if [[ "${MSYSTEM:-}" != UCRT64 || -z "${MINGW_PREFIX:-}" ]]; then
  echo 'Run this script in an MSYS2 UCRT64 shell.' >&2
  exit 1
fi
prefix="$(cygpath -am "${QT_BUILD_PREFIX:-$HOME/.cache/arcana-qt-6.8.3-windows-minimal-v1}")"
work="$(cygpath -am "${QT_BUILD_WORK:-${RUNNER_TEMP:-/tmp}/arcana-qt-windows-minimal-v1}")"
mingw_prefix="$(cygpath -am "$MINGW_PREFIX")"
export PATH="$(cygpath -u "$mingw_prefix")/bin:$PATH"
# 同时绑定编译器、链接器、运行库软件包和脚本内容，防止复用旧 ABI 或旧配置缓存。
toolchain_id="$({ gcc --version; g++ --version; ld --version; pacman -Q | sort; sha256sum < "${BASH_SOURCE[0]}"; } | sha256sum | cut -d ' ' -f 1)"
if [[ ! -f "$prefix/.complete" || "$(cat "$prefix/.complete")" != "$toolchain_id" ]]; then
  mkdir -p "$work" "$prefix/share/licenses/qtbase"
  if [[ ! -f "$work/qtbase.tar.xz" ]]; then
    curl --fail --location --retry 3 \
      https://download.qt.io/archive/qt/6.8/6.8.3/submodules/qtbase-everywhere-src-6.8.3.tar.xz \
      -o "$work/qtbase.tar.xz.part"
    mv "$work/qtbase.tar.xz.part" "$work/qtbase.tar.xz"
  fi
  printf '%s  %s\n' 56001b905601bb9023d399f3ba780d7fa940f3e4861e496a7c490331f49e0b80 "$work/qtbase.tar.xz" | sha256sum --check --strict
  tar --force-local -xf "$work/qtbase.tar.xz" -C "$work"
  source_dir="$work/qtbase-everywhere-src-6.8.3"
  # 功能宏变化后，CMake --fresh 不会清理旧 moc 输出，必须换掉旧构建目录。
  if [[ ! -f "$work/.build-id" || "$(cat "$work/.build-id")" != "$toolchain_id" ]]; then
    rm -rf "$work/qtbase-build"
    printf '%s\n' "$toolchain_id" > "$work/.build-id"
  fi
  # 保留字体整形、彩色字形和光栅 QWidget；使用内置第三方库减少外部 DLL。
  # Qt 6.8 的 DirectWrite 字体加载依赖 Direct2D，不能随 GPU 后端一起禁用。
  cmake --fresh -S "$source_dir" -B "$work/qtbase-build" -GNinja \
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
  rm -rf "$prefix/share/licenses/qtbase"
  mkdir -p "$prefix/share/licenses/qtbase"
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
export PATH="$(cygpath -u "$prefix")/bin:$(cygpath -u "$mingw_prefix")/bin:$PATH"
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
