.PHONY: build overlay overlay-qt tts desktop desktop-native run check clean

GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)
VERSION ?=
OUT_DIR ?= bin
BUILD = GOOS="$(GOOS)" GOARCH="$(GOARCH)" VERSION="$(VERSION)" OUT_DIR="$(OUT_DIR)" bash scripts/build.sh

# 默认构建仍为独立且不依赖 CGO 的 TUI。
build overlay overlay-qt tts desktop desktop-native:
	$(BUILD) $@

run:
	go run ./cmd/arcana-world

check:
	go test -race ./...
	go vet ./...

clean:
	rm -f "$(OUT_DIR)/arcana-world" "$(OUT_DIR)/arcana-world.exe"
	rm -rf "$(OUT_DIR)/libexec" "$(OUT_DIR)/.overlay-qt-build"
