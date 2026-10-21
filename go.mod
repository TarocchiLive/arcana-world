module arcana-world

go 1.26.8

require (
	charm.land/bubbles/v2 v2.2.1
	charm.land/bubbletea/v2 v2.0.9
	charm.land/lipgloss/v2 v2.0.6
	github.com/andybalholm/brotli v1.2.4
	github.com/charmbracelet/ultraviolet v0.0.0-20260811164956-006e29f97886
	github.com/charmbracelet/x/ansi v0.11.8
	github.com/charmbracelet/x/term v0.2.2
	github.com/ebitengine/oto/v3 v3.5.0
	github.com/ebitengine/purego v0.11.0
	github.com/godbus/dbus/v5 v5.2.2
	github.com/gorilla/websocket v1.5.3
	github.com/hajimehoshi/go-mp3 v0.3.4
	github.com/jfreymuth/oggvorbis v1.0.5
	github.com/mewkiz/flac v1.0.13
	github.com/rivo/uniseg v0.4.7
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e
	github.com/wujunwei928/edge-tts-go v0.0.3-0.20260613042911-e8c68c41cd80
	github.com/zalando/go-keyring v0.2.8
	go.etcd.io/bbolt v1.5.0
	golang.org/x/image v0.46.0
	golang.org/x/mod v0.42.0
	golang.org/x/sys v0.48.0
	google.golang.org/protobuf v1.36.12
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/atotto/clipboard v0.1.4 // indirect
	github.com/charmbracelet/colorprofile v0.4.3 // indirect
	github.com/charmbracelet/x/termios v0.1.1 // indirect
	github.com/charmbracelet/x/windows v0.2.2 // indirect
	github.com/clipperhouse/displaywidth v0.11.0 // indirect
	github.com/clipperhouse/uax29/v2 v2.7.0 // indirect
	github.com/danieljoos/wincred v1.2.3 // indirect
	github.com/go-resty/resty/v2 v2.17.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/icza/bitio v1.1.0 // indirect
	github.com/jfreymuth/pulse v0.1.3 // indirect
	github.com/jfreymuth/vorbis v1.0.2 // indirect
	github.com/lucasb-eyer/go-colorful v1.4.1 // indirect
	github.com/mattn/go-runewidth v0.0.30 // indirect
	github.com/mewkiz/pkg v0.0.0-20250417130911-3f050ff8c56d // indirect
	github.com/mewpkg/term v0.0.0-20241026122259-37a80af23985 // indirect
	github.com/muesli/cancelreader v0.2.2 // indirect
	github.com/xo/terminfo v1.2.0 // indirect
	golang.org/x/net v0.59.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
)

// 固定 https://github.com/charmbracelet/lipgloss/pull/732 的补丁，避免 32 位平台截断样式标记。
replace charm.land/lipgloss/v2 => github.com/repost-sh/lipgloss/v2 v2.0.7-0.20260818135826-488b5b63a693

// 固定 e8c68c41cd80 的本地端点补丁；上游仍使用已拒绝合成请求的旧接口。
// 协议变更：https://github.com/rany2/edge-tts/pull/491
replace github.com/wujunwei928/edge-tts-go => ./third_party/edge-tts-go
