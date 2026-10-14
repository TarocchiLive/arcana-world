#!/usr/bin/env bash
set -euo pipefail

# 在发行基线 Ubuntu 24.04 上构建，避免提高用户系统的 glibc 要求。
prefix="${QT_BUILD_PREFIX:-$HOME/.cache/arcana-qt-6.8.3-layer-6.3.6}"
work="${RUNNER_TEMP:-/tmp}/arcana-qt-sources"
sudo apt-get update
sudo apt-get install -y build-essential cmake ninja-build pkg-config patchelf curl \
  libwayland-dev wayland-protocols libpango1.0-dev libcairo2-dev libasound2-dev \
  libfontconfig1-dev libfreetype-dev libgl1-mesa-dev libegl1-mesa-dev libxkbcommon-dev \
  libxkbcommon-x11-dev libxcb1-dev libxcb-cursor-dev libxcb-icccm4-dev \
  libxcb-image0-dev libxcb-keysyms1-dev libxcb-randr0-dev libxcb-render-util0-dev \
  libxcb-shape0-dev libxcb-shm0-dev libxcb-sync-dev libxcb-xfixes0-dev libxcb-xinerama0-dev \
  libxcb-xkb-dev libx11-xcb-dev libdbus-1-dev libinput-dev libudev-dev
if [[ ! -f "$prefix/.complete" ]]; then
  mkdir -p "$work" "$prefix/share/licenses"
  for module in qtbase qtshadertools qtdeclarative qtwayland; do
    curl --fail --location --retry 3 "https://download.qt.io/archive/qt/6.8/6.8.3/submodules/${module}-everywhere-src-6.8.3.tar.xz" -o "$work/$module.tar.xz"
    tar -xf "$work/$module.tar.xz" -C "$work"
  done
  cmake -S "$work/qtbase-everywhere-src-6.8.3" -B "$work/qtbase-build" -GNinja \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" \
    -DQT_BUILD_TESTS=OFF -DQT_BUILD_EXAMPLES=OFF -DBUILD_SHARED_LIBS=ON \
    -DFEATURE_xcb=ON -DFEATURE_opengl=ON
  cmake --build "$work/qtbase-build" --parallel "${BUILD_JOBS:-2}"
  cmake --install "$work/qtbase-build"
  # LayerShellQt 的构建系统同时要求 Qml；浮层本身仍使用 Widgets。
  for module in qtshadertools qtdeclarative qtwayland; do
    cmake -S "$work/$module-everywhere-src-6.8.3" -B "$work/$module-build" -GNinja \
      -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" -DCMAKE_PREFIX_PATH="$prefix" \
      -DQT_BUILD_TESTS=OFF -DQT_BUILD_EXAMPLES=OFF
    cmake --build "$work/$module-build" --parallel "${BUILD_JOBS:-2}"
    cmake --install "$work/$module-build"
  done
  curl --fail --location --retry 3 https://download.kde.org/stable/frameworks/6.12/extra-cmake-modules-6.12.0.tar.xz -o "$work/ecm.tar.xz"
  tar -xf "$work/ecm.tar.xz" -C "$work"
  cmake -S "$work/extra-cmake-modules-6.12.0" -B "$work/ecm-build" -DCMAKE_INSTALL_PREFIX="$prefix" -DBUILD_TESTING=OFF
  cmake --install "$work/ecm-build"
  curl --fail --location --retry 3 https://download.kde.org/stable/plasma/6.3.6/layer-shell-qt-6.3.6.tar.xz -o "$work/layer.tar.xz"
  tar -xf "$work/layer.tar.xz" -C "$work"
  cmake -S "$work/layer-shell-qt-6.3.6" -B "$work/layer-build" -GNinja \
    -DCMAKE_BUILD_TYPE=Release -DCMAKE_INSTALL_PREFIX="$prefix" -DCMAKE_PREFIX_PATH="$prefix" \
    -DBUILD_TESTING=OFF -DBUILD_EXAMPLES=OFF
  cmake --build "$work/layer-build" --parallel "${BUILD_JOBS:-2}"
  cmake --install "$work/layer-build"
  for module in qtbase qtshadertools qtdeclarative qtwayland; do
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
