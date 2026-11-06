// Package overlay 管理原生浮层的配置、控制连接和子进程，不依赖图形库。
//
// 浮层按完整显示器边界定位，更新文字不改变窗口尺寸或焦点。
// macOS 使用 AppKit 面板；Windows 支持 amd64、arm64 的分层窗口。
// Linux 原生浮层要求 Wayland layer-shell；精美浮层可通过 GNOME Shell 扩展管理窗口。
// 所有平台均不保证覆盖锁屏、安全桌面或独占全屏。
package overlay
