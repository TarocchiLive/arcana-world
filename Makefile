.PHONY: build overlay tts desktop run check clean

OVERLAY_GOOS := $(shell go env GOOS)
OVERLAY_CGO := 1
OVERLAY_TAGS :=
AUDIO_CGO := 0
EXE_SUFFIX :=
ifeq ($(OVERLAY_GOOS),windows)
OVERLAY_CGO := 0
EXE_SUFFIX := .exe
endif
ifeq ($(OVERLAY_GOOS),linux)
OVERLAY_TAGS := -tags wayland
AUDIO_CGO := 1
endif

build:
	go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o bin/arcana-world$(EXE_SUFFIX) ./cmd/arcana-world

# 原生浮层：macOS 使用 AppKit，Windows 使用 Win32，Linux 使用 layer-shell。
overlay:
	CGO_ENABLED=$(OVERLAY_CGO) go build $(OVERLAY_TAGS) -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o bin/libexec/arcana-world-overlay$(EXE_SUFFIX) ./cmd/arcana-world-overlay

tts:
	CGO_ENABLED=$(AUDIO_CGO) go build -tags tts_audio -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o bin/libexec/arcana-world-tts$(EXE_SUFFIX) ./cmd/arcana-world-tts

# 配套构建；默认 build 仍只构建不依赖图形运行时的 TUI。
desktop: build overlay tts

run:
	go run ./cmd/arcana-world

check:
	go test -race ./...
	go vet ./...

clean:
	rm -f bin/arcana-world bin/arcana-world.exe bin/libexec/arcana-world-overlay bin/libexec/arcana-world-overlay.exe bin/arcana-world-overlay bin/arcana-world-overlay.exe bin/libexec/arcana-world-tts bin/libexec/arcana-world-tts.exe
