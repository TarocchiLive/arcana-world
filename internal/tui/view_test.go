package tui

import (
	"context"
	"strings"
	"testing"

	"arcana-world/internal/store"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestLongStatusRemainsCompleteAcrossResize(t *testing.T) {
	s, err := store.Open(t.TempDir(), store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	m, err := New(context.Background(), s)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Close() })
	message := "STATUS_BEGIN 开播失败：自动连接 OBS 失败，尚未开播：请在 OBS 中打开「工具」→「WebSocket 服务器设置」，确认已勾选「开启 WebSocket 服务器」，并核对密码是否正确。\n连接地址：ws://localhost:4455/" + strings.Repeat("endpoint", 12) + " STATUS_END"
	compact := func(s string) string { return strings.Join(strings.Fields(s), "") }
	for _, width := range []int{120, 64, 100} {
		m.Update(tea.WindowSizeMsg{Width: width, Height: 32})
		m.status = message
		screen := ansi.Strip(m.View())
		start, end := strings.Index(screen, "STATUS_BEGIN"), strings.Index(screen, "STATUS_END")
		if start < 0 || end < start {
			t.Fatalf("width %d: status was truncated: %s", width, screen)
		}
		shown := screen[start : end+len("STATUS_END")]
		if compact(shown) != compact(message) {
			t.Fatalf("width %d: wrapped status lost text: %q", width, shown)
		}
		for _, line := range strings.Split(shown, "\n") {
			if ansi.StringWidth(strings.TrimSpace(line)) > width-4 {
				t.Fatalf("width %d: status line exceeds available width: %q", width, line)
			}
		}
		if lipgloss.Height(screen) > m.height {
			t.Fatalf("width %d: wrapped status pushed the view beyond the terminal height", width)
		}
		m.status = "SHORT_STATUS"
		if short := ansi.Strip(m.View()); !strings.Contains(short, "SHORT_STATUS") || lipgloss.Height(short) > m.height {
			t.Fatalf("width %d: short status failed after wrapping", width)
		}
	}
}
