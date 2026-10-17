#!/usr/bin/env bash
set -euo pipefail

# 在发行基线 Ubuntu 24.04 上构建，避免提高用户系统的 glibc 要求。
prefix="${QT_BUILD_PREFIX:-$HOME/.cache/arcana-qt-6.8.3-layer-6.3.6-minimal-v2}"
work="${RUNNER_TEMP:-/tmp}/arcana-qt-sources-minimal-v2"
sudo apt-get update
# Pango/Cairo 供原生浮层使用，ALSA 供音频使用；Qt 本身仅需字体、D-Bus 和 Wayland。
sudo apt-get install -y build-essential cmake ninja-build pkg-config patchelf curl patch \
  libwayland-dev wayland-protocols libpango1.0-dev libcairo2-dev libasound2-dev \
  libfontconfig1-dev libfreetype-dev libxkbcommon-dev libdbus-1-dev
if [[ ! -f "$prefix/.complete" ]]; then
  mkdir -p "$work" "$prefix/share/licenses"
  for module in qtbase qtwayland; do
    curl --fail --location --retry 3 "https://download.qt.io/archive/qt/6.8/6.8.3/submodules/${module}-everywhere-src-6.8.3.tar.xz" -o "$work/$module.tar.xz"
    tar -xf "$work/$module.tar.xz" -C "$work"
  done
  # 保留 Widgets、字体整形、图像和 D-Bus；raster Wayland 不需要 GPU、X11 或裸设备输入。
  # 功能名依据 Qt 6.8.3 的 configure.cmake 和 src/gui/configure.cmake。
  cmake -S "$work/qtbase-everywhere-src-6.8.3" -B "$work/qtbase-build" -GNinja \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" \
    -DQT_BUILD_TESTS=OFF -DQT_BUILD_EXAMPLES=OFF -DBUILD_SHARED_LIBS=ON \
    -DFEATURE_gui=ON -DFEATURE_widgets=ON -DFEATURE_dbus=ON \
    -DFEATURE_fontconfig=ON -DFEATURE_freetype=ON -DFEATURE_xkbcommon=ON -DFEATURE_wayland=ON \
    -DFEATURE_xcb=OFF -DFEATURE_xlib=OFF -DFEATURE_xkbcommon_x11=OFF \
    -DINPUT_opengl=no -DFEATURE_opengl=OFF -DFEATURE_egl=OFF -DFEATURE_vulkan=OFF \
    -DFEATURE_eglfs=OFF -DFEATURE_linuxfb=OFF -DFEATURE_libinput=OFF -DFEATURE_libudev=OFF -DFEATURE_evdev=OFF \
    -DFEATURE_network=OFF -DFEATURE_sql=OFF -DFEATURE_xml=OFF \
    -DFEATURE_concurrent=OFF -DFEATURE_printsupport=OFF -DFEATURE_testlib=OFF
  cmake --build "$work/qtbase-build" --parallel "${BUILD_JOBS:-2}"
  cmake --install "$work/qtbase-build"
  # QtWayland 的 Quick 是可选依赖；wayland-server 才是 compositor 的实际功能开关。
  # https://github.com/qt/qtwayland/blob/v6.8.3/src/configure.cmake
  cmake -S "$work/qtwayland-everywhere-src-6.8.3" -B "$work/qtwayland-build" -GNinja \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" -DCMAKE_PREFIX_PATH="$prefix" \
    -DQT_BUILD_TESTS=OFF -DQT_BUILD_EXAMPLES=OFF \
    -DFEATURE_wayland_client=ON -DFEATURE_wayland_server=OFF -DFEATURE_wayland_egl=OFF
  cmake --build "$work/qtwayland-build" --parallel "${BUILD_JOBS:-2}"
  cmake --install "$work/qtwayland-build"
  curl --fail --location --retry 3 https://download.kde.org/stable/frameworks/6.12/extra-cmake-modules-6.12.0.tar.xz -o "$work/ecm.tar.xz"
  tar -xf "$work/ecm.tar.xz" -C "$work"
  cmake -S "$work/extra-cmake-modules-6.12.0" -B "$work/ecm-build" -DCMAKE_INSTALL_PREFIX="$prefix" -DBUILD_TESTING=OFF
  cmake --install "$work/ecm-build"
  curl --fail --location --retry 3 https://download.kde.org/stable/plasma/6.3.6/layer-shell-qt-6.3.6.tar.xz -o "$work/layer.tar.xz"
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
  cmake --install "$work/layer-build"
  for module in qtbase qtwayland; do
    cp -R "$work/$module-everywhere-src-6.8.3/LICENSES" "$prefix/share/licenses/$module"
  done
  cp -R "$work/layer-shell-qt-6.3.6/LICENSES" "$prefix/share/licenses/layer-shell-qt"
  touch "$prefix/.complete"
fi
if [[ -n "${GITHUB_ENV:-}" ]]; then
  echo "CMAKE_PREFIX_PATH=$prefix" >> "$GITHUB_ENV"
  echo "QTPATHS=$prefix/bin/qtpaths" >> "$GITHUB_ENV"
  echo "QT_LICENSE_DIR=$prefix/share/licenses" >> "$GITHUB_ENV"
  echo "$prefix/bin" >> "$GITHUB_PATH"
fi
