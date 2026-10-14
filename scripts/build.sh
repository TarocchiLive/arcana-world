#!/usr/bin/env bash
set -euo pipefail

# 默认目标仅构建不依赖 CGO 的 TUI；完整桌面包必须包含两种浮层。
export GOOS="${GOOS:-$(go env GOOS)}"
export GOARCH="${GOARCH:-$(go env GOARCH)}"
out_dir="${OUT_DIR:-bin}"
version="${VERSION:-}"
suffix=
overlay_cgo=1
audio_cgo=0
overlay_tags=
case "$GOOS" in
  windows) suffix=.exe; overlay_cgo=0 ;;
  linux) overlay_tags=wayland; audio_cgo=1 ;;
esac
ldflags='-s -w -buildid='
tui_ldflags="$ldflags"
if [[ -n "$version" ]]; then
  tui_ldflags+=" -X main.version=${version#v}"
fi

build_qt() {
  if [[ "$GOOS" != "$(go env GOHOSTOS)" || "$GOARCH" != "$(go env GOHOSTARCH)" ]]; then
    echo 'Elegant overlay requires a matching host architecture and C++ toolchain' >&2
    exit 2
  fi
  local build_dir install_dir qt_ldflags
  mkdir -p "$out_dir/libexec" "$out_dir/.overlay-qt-build"
  build_dir="$(cd "$out_dir/.overlay-qt-build" && pwd)"
  install_dir="$(cd "$out_dir/libexec" && pwd)"
  cmake -S internal/overlay/qt/cpp -B "$build_dir" -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_INSTALL_PREFIX="$install_dir" -DCMAKE_INSTALL_LIBDIR=.
  cmake --build "$build_dir" --config Release --parallel "${BUILD_JOBS:-2}"
  cmake --install "$build_dir" --config Release
  qt_ldflags="$ldflags"
  case "$GOOS" in
    darwin) qt_ldflags+=' -extldflags=-Wl,-rpath,@loader_path' ;;
    linux) qt_ldflags+=' -extldflags=-Wl,-rpath,$ORIGIN' ;;
  esac
  CGO_ENABLED=1 CGO_LDFLAGS="${CGO_LDFLAGS:-} \"-L$install_dir\"" \
    go build -tags qt -trimpath -buildvcs=false -ldflags="$qt_ldflags" \
    -o "$install_dir/arcana-world-overlay-qt$suffix" ./cmd/arcana-world-overlay
  python3 scripts/deploy-qt.py "$install_dir"
}

build() {
  local component="$1" cgo tags output flags command
  case "$component" in
    build) cgo=0; tags=; output="arcana-world$suffix"; flags="$tui_ldflags"; command=arcana-world ;;
    overlay) cgo="$overlay_cgo"; tags="$overlay_tags"; output="libexec/arcana-world-overlay-native$suffix"; flags="$ldflags"; command=arcana-world-overlay ;;
    tts) cgo="$audio_cgo"; tags=tts_audio; output="libexec/arcana-world-tts$suffix"; flags="$ldflags"; command=arcana-world-tts ;;
    *) echo 'Usage: bash scripts/build.sh {build|overlay|overlay-qt|tts|desktop|desktop-native}' >&2; exit 2 ;;
  esac
  mkdir -p "$(dirname "$out_dir/$output")"
  CGO_ENABLED="$cgo" go build -tags "$tags" -trimpath -buildvcs=false \
    -ldflags="$flags" -o "$out_dir/$output" "./cmd/$command"
}

case "${1:-build}" in
  overlay-qt) build_qt ;;
  desktop|desktop-native)
    build build
    build overlay
    build tts
    if [[ "$1" == desktop ]]; then
      build_qt
    fi
    ;;
  *) build "${1:-build}" ;;
esac
