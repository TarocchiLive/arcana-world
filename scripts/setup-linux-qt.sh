#!/usr/bin/env bash
set -euo pipefail

if [[ "${1:-}" == -h || "${1:-}" == --help ]]; then
  printf '%s\n' "Usage: bash ${BASH_SOURCE[0]} [--cache-info]" \
    'Build the native Qt SDK, or print its cache prefix and toolchain identity.' \
    'Environment: QT_BUILD_PREFIX, QT_BUILD_WORK, BUILD_JOBS, CC, CXX, NATIVE_DEPS.' \
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

# 在发行基线 Ubuntu 24.04 上构建，避免提高用户系统的 glibc 要求。
script_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
prefix="${QT_BUILD_PREFIX:-$HOME/.cache/arcana-qt-6.8.3-layer-6.3.6-minimal-v3}"
work="${QT_BUILD_WORK:-${RUNNER_TEMP:-/tmp}/arcana-qt-sources-minimal-v3}"
# 缓存查询也先安装依赖，保证不可变缓存键与实际构建的软件包版本一致。
packages=(build-essential cmake ninja-build pkg-config patchelf curl patch python3
  libwayland-dev wayland-protocols libpango1.0-dev libcairo2-dev libasound2-dev
  libfontconfig1-dev libfreetype-dev libxkbcommon-dev libdbus-1-dev)
sudo apt-get update >&2
sudo apt-get install -y "${packages[@]}" >&2
prefix="$(python3 "$script_dir/qt-build-paths.py" canonical "$prefix")"
work="$(python3 "$script_dir/qt-build-paths.py" canonical "$work")"
python3 "$script_dir/qt-build-paths.py" check "$prefix" "$work"
export CC="${CC:-gcc}" CXX="${CXX:-g++}"
# 同一指纹同时用于 .complete 和 CI 缓存，不允许配置变化后覆盖旧安装。
toolchain_id="$({
  set -e
  if [[ -n "${CMAKE_TOOLCHAIN_FILE:-}" ]]; then sha256sum < "$CMAKE_TOOLCHAIN_FILE"; fi
  "$CC" --version; "$CXX" --version; ld --version; cmake --version; ninja --version
  dpkg-query -W -f='${Package} ${Version}\n' "${packages[@]}" libc6-dev libstdc++6 | LC_ALL=C sort
  printf '%s\n' "$prefix" "$CC" "$CXX" "${CFLAGS:-}" "${CXXFLAGS:-}" "${CPPFLAGS:-}" "${LDFLAGS:-}"
  printf '%s\n' "${CMAKE_TOOLCHAIN_FILE:-}"
  sha256sum < "${BASH_SOURCE[0]}"; sha256sum < "$script_dir/qt-build-paths.py"
} | sha256sum | cut -d ' ' -f 1)"
# 依赖清单中有与本脚本一致的预编译 SDK 时直接使用；缓存键随包内容变化。
case "$(uname -m)" in
  x86_64) goarch=amd64 ;;
  aarch64|arm64) goarch=arm64 ;;
  *) goarch="$(uname -m)" ;;
esac
status=0
pack="$(python3 "$script_dir/native-deps.py" resolve qt linux "$goarch")" || status=$?
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
  python3 "$script_dir/qt-build-paths.py" reset "$prefix" "$work"
  python3 "$script_dir/native-deps.py" fetch qt linux "$goarch" "$prefix"
  printf '%s\n' "$toolchain_id" > "$prefix/.complete"
fi
if [[ ! -f "$prefix/.complete" || "$(cat "$prefix/.complete")" != "$toolchain_id" ]]; then
  python3 "$script_dir/qt-build-paths.py" reset "$prefix" "$work"
  mkdir -p "$prefix/share/licenses"
  for module in qtbase qtwayland; do
    curl --fail --location --retry 3 "https://download.qt.io/archive/qt/6.8/6.8.3/submodules/${module}-everywhere-src-6.8.3.tar.xz" -o "$work/$module.tar.xz"
    case "$module" in
      qtbase) checksum=56001b905601bb9023d399f3ba780d7fa940f3e4861e496a7c490331f49e0b80 ;;
      qtwayland) checksum=20fe385887d21190165a3180c17dcfc8b9a0e1da4ec76865b6334bdc709994b0 ;;
    esac
    printf '%s  %s\n' "$checksum" "$work/$module.tar.xz" | sha256sum --check --strict
    tar -xf "$work/$module.tar.xz" -C "$work"
  done
  # 保留 Widgets、字体整形、图像和 D-Bus；raster Wayland 不需要 GPU、X11 或裸设备输入。
  # 功能名依据 Qt 6.8.3 的 configure.cmake 和 src/gui/configure.cmake。
  # 浮层仅使用自绘 QWidget、QTextLayout 与 QPainter，不使用编辑器、对话框或样式表。
  # Fontconfig 要求 system_freetype；Ubuntu 基线的 FreeType 不依赖 ICU/HarfBuzz。
  # 使用 Qt 内置 HarfBuzz 保留整形，避免系统 HarfBuzz 额外带入 GLib/ICU。
  cmake -S "$work/qtbase-everywhere-src-6.8.3" -B "$work/qtbase-build" -GNinja \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" \
    -DCMAKE_C_COMPILER="$CC" -DCMAKE_CXX_COMPILER="$CXX" \
    -DQT_BUILD_TESTS=OFF -DQT_BUILD_EXAMPLES=OFF -DBUILD_SHARED_LIBS=ON \
    -DFEATURE_optimize_size=ON -DFEATURE_gui=ON -DFEATURE_widgets=ON -DFEATURE_dbus=ON \
    -DFEATURE_icu=OFF -DFEATURE_glib=OFF \
    -DFEATURE_fontconfig=ON -DFEATURE_freetype=ON -DFEATURE_system_freetype=ON \
    -DFEATURE_harfbuzz=ON -DFEATURE_system_harfbuzz=OFF \
    -DFEATURE_xkbcommon=ON -DFEATURE_wayland=ON \
    -DFEATURE_xcb=OFF -DFEATURE_xlib=OFF -DFEATURE_xkbcommon_x11=OFF \
    -DINPUT_opengl=no -DFEATURE_opengl=OFF -DFEATURE_egl=OFF -DFEATURE_vulkan=OFF \
    -DFEATURE_eglfs=OFF -DFEATURE_linuxfb=OFF -DFEATURE_libinput=OFF -DFEATURE_libudev=OFF -DFEATURE_evdev=OFF \
    -DFEATURE_network=OFF -DFEATURE_sql=OFF -DFEATURE_xml=OFF \
    -DFEATURE_concurrent=OFF -DFEATURE_printsupport=OFF -DFEATURE_testlib=OFF \
    -DFEATURE_itemviews=OFF -DFEATURE_graphicsview=OFF -DFEATURE_mainwindow=OFF \
    -DFEATURE_dialog=OFF -DFEATURE_textedit=OFF -DFEATURE_calendarwidget=OFF \
    -DFEATURE_filesystemmodel=OFF -DFEATURE_pdf=OFF -DFEATURE_style_stylesheet=OFF \
    -DFEATURE_textmarkdownreader=OFF -DFEATURE_textmarkdownwriter=OFF -DFEATURE_textodfwriter=OFF
  cmake --build "$work/qtbase-build" --parallel "${BUILD_JOBS:-2}"
  cmake --install "$work/qtbase-build" --strip
  # QtWayland 的 Quick 是可选依赖；wayland-server 才是 compositor 的实际功能开关。
  # https://github.com/qt/qtwayland/blob/v6.8.3/src/configure.cmake
  cmake -S "$work/qtwayland-everywhere-src-6.8.3" -B "$work/qtwayland-build" -GNinja \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" -DCMAKE_PREFIX_PATH="$prefix" \
    -DQT_BUILD_TESTS=OFF -DQT_BUILD_EXAMPLES=OFF \
    -DFEATURE_wayland_client=ON -DFEATURE_wayland_server=OFF -DFEATURE_wayland_egl=OFF
  cmake --build "$work/qtwayland-build" --parallel "${BUILD_JOBS:-2}"
  cmake --install "$work/qtwayland-build" --strip
  curl --fail --location --retry 3 https://download.kde.org/stable/frameworks/6.12/extra-cmake-modules-6.12.0.tar.xz -o "$work/ecm.tar.xz"
  printf '%s  %s\n' e54e87365e193c806966e9698320bde81981219574721ff1970a98163c8ed5a0 "$work/ecm.tar.xz" | sha256sum --check --strict
  tar -xf "$work/ecm.tar.xz" -C "$work"
  cmake -S "$work/extra-cmake-modules-6.12.0" -B "$work/ecm-build" -DCMAKE_INSTALL_PREFIX="$prefix" -DBUILD_TESTING=OFF
  cmake --install "$work/ecm-build"
  curl --fail --location --retry 3 https://download.kde.org/stable/plasma/6.3.6/layer-shell-qt-6.3.6.tar.xz -o "$work/layer.tar.xz"
  printf '%s  %s\n' bd8d1a7971fb720b2f1475e421e4e425992d0ec22c106047081c7da21cd0b2fc "$work/layer.tar.xz" | sha256sum --check --strict
  tar -xf "$work/layer.tar.xz" -C "$work"
  # 6.3.6 没有 BUILD_QML 开关：顶层强制 Qml，并无条件构建 declarative 和 Qml 测试。
  # 明确修补这三处入口及 QML 宏引入，仅保留原样的 C++ 接口与 layer-shell 插件。
  # https://github.com/KDE/layer-shell-qt/blob/v6.3.6/CMakeLists.txt
  # https://github.com/KDE/layer-shell-qt/blob/v6.3.6/src/CMakeLists.txt
  # 精确匹配固定版本源码，失败即停止；不以无效 CMake 变量掩盖强制依赖。
  patch --batch --fuzz=0 -d "$work/layer-shell-qt-6.3.6" -p1 <<'PATCH'
--- a/CMakeLists.txt
+++ b/CMakeLists.txt
@@ -31,9 +31,8 @@
 include(KDEClangFormat)
 include(ECMQtDeclareLoggingCategory)
-include(ECMQmlModule)
 include(KDEGitCommitHooks)
 
-find_package(Qt6 ${QT_MIN_VERSION} CONFIG REQUIRED COMPONENTS WaylandClient Qml)
+find_package(Qt6 ${QT_MIN_VERSION} CONFIG REQUIRED COMPONENTS WaylandClient)
 find_package(WaylandScanner REQUIRED)
 find_package(Wayland 1.3 COMPONENTS Client Server)
 find_package(WaylandProtocols REQUIRED)
@@ -63,3 +62,2 @@
 add_subdirectory(src)
-add_subdirectory(tests)
 
--- a/src/CMakeLists.txt
+++ b/src/CMakeLists.txt
@@ -80,3 +80,2 @@
 )
 
-add_subdirectory(declarative)
PATCH
  cmake -S "$work/layer-shell-qt-6.3.6" -B "$work/layer-build" -GNinja \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" -DCMAKE_PREFIX_PATH="$prefix" \
    -DBUILD_SHARED_LIBS=ON -DBUILD_TESTING=OFF
  cmake --build "$work/layer-build" --parallel "${BUILD_JOBS:-2}"
  cmake --install "$work/layer-build" --strip
  for module in qtbase qtwayland; do
    cp -R "$work/$module-everywhere-src-6.8.3/LICENSES" "$prefix/share/licenses/$module"
  done
  cp -R "$work/layer-shell-qt-6.3.6/LICENSES" "$prefix/share/licenses/layer-shell-qt"
  # 内嵌 HarfBuzz 的版权声明随运行库分发，通用许可文本不能替代它。
  mkdir -p "$prefix/share/licenses/harfbuzz-ng"
  cp "$work/qtbase-everywhere-src-6.8.3/src/3rdparty/harfbuzz-ng/"{COPYING,qt_attribution.json} \
    "$prefix/share/licenses/harfbuzz-ng/"
  printf '%s\n' "$toolchain_id" > "$prefix/.complete"
fi
if [[ -n "${GITHUB_ENV:-}" ]]; then
  echo "CMAKE_PREFIX_PATH=$prefix" >> "$GITHUB_ENV"
  echo "QTPATHS=$prefix/bin/qtpaths" >> "$GITHUB_ENV"
  echo "QT_LICENSE_DIR=$prefix/share/licenses" >> "$GITHUB_ENV"
  echo "$prefix/bin" >> "$GITHUB_PATH"
fi
