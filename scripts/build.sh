#!/usr/bin/env bash
set -euo pipefail

# Shared by Make, CI, and releases; native helpers require target-platform toolchains.
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

build() {
  local component="$1"
  local cgo tags output flags
  case "$component" in
    build) cgo=0; tags=; output="arcana-world$suffix"; flags="$tui_ldflags" ;;
    overlay) cgo="$overlay_cgo"; tags="$overlay_tags"; output="libexec/arcana-world-overlay$suffix"; flags="$ldflags" ;;
    tts) cgo="$audio_cgo"; tags=tts_audio; output="libexec/arcana-world-tts$suffix"; flags="$ldflags" ;;
    *) echo "Usage: bash scripts/build.sh {build|overlay|tts|desktop}" >&2; exit 2 ;;
  esac
  local command="${output##*/}"
  command="${command%.exe}"
  mkdir -p "$(dirname "$out_dir/$output")"
  CGO_ENABLED="$cgo" go build -tags "$tags" -trimpath -buildvcs=false \
    -ldflags="$flags" -o "$out_dir/$output" "./cmd/$command"
}

if [[ "${1:-build}" == desktop ]]; then
  build build
  build overlay
  build tts
else
  build "${1:-build}"
fi
