.PHONY: build overlay run check clean

OVERLAY_GOOS := $(shell go env GOOS)
OVERLAY_CGO := 1
OVERLAY_TAGS :=
EXE_SUFFIX :=
ifeq ($(OVERLAY_GOOS),windows)
OVERLAY_CGO := 0
EXE_SUFFIX := .exe
endif
ifeq ($(OVERLAY_GOOS),linux)
OVERLAY_TAGS := -tags wayland
endif

build:
	go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o bin/arcana-world$(EXE_SUFFIX) ./cmd/arcana-world

# 原生浮层：macOS 使用 AppKit，Windows 使用 Win32，Linux 使用 layer-shell。
overlay:
	CGO_ENABLED=$(OVERLAY_CGO) go build $(OVERLAY_TAGS) -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o bin/arcana-overlay$(EXE_SUFFIX) ./cmd/arcana-overlay

run:
	go run ./cmd/arcana-world

check:
	go test -race ./...
	go vet ./...

clean:
	rm -f bin/arcana-world bin/arcana-world.exe bin/arcana-overlay bin/arcana-overlay.exe
