package tui

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"arcana-world/internal/store"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
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
		m.warnStatus(message)
		screen := ansi.Strip(m.View().Content)
		// 读取可见卡片，不读取浮层旁边的底层页面。
		card := m.noticeLayer
		var body strings.Builder
		lines := strings.Split(screen, "\n")
		for row := card.y + 2; row < card.y+card.height-1; row++ {
			body.WriteString(ansi.Cut(lines[row], card.x+2, card.x+card.width-2))
			body.WriteByte('\n')
		}
		start, end := strings.Index(body.String(), "STATUS_BEGIN"), strings.Index(body.String(), "STATUS_END")
		if start < 0 || end < start {
			t.Fatalf("width %d: status was truncated: %s", width, screen)
		}
		shown := body.String()[start : end+len("STATUS_END")]
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
		m.setStatus("SHORT_STATUS")
		if short := ansi.Strip(m.View().Content); !strings.Contains(short, "SHORT_STATUS") || lipgloss.Height(short) > m.height {
			t.Fatalf("width %d: short status failed after wrapping", width)
		}
	}
}

func TestNotificationScrollAndDismissDoNotActivateCoveredActions(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.Update(tea.WindowSizeMsg{Width: 64, Height: 24})
	m.page = settingsPage
	var message strings.Builder
	for i := range 24 {
		fmt.Fprintf(&message, "DETAIL-%02d 连接失败，请检查服务地址。\n", i)
	}
	m.warn(strings.TrimSpace(message.String()))
	m.View()
	card := m.noticeLayer
	clicked := false
	for _, target := range m.mouseTargets {
		x := max(target.x, card.x+1)
		if target.kind == "action" && x < min(target.x+target.width, card.x+card.width-1) &&
			target.y > card.y && target.y < card.y+card.height-1 {
			clicked = true
			m.Update(tea.MouseClickMsg{X: x, Y: target.y, Button: tea.MouseLeft})
			if m.mode != "" || m.busy {
				t.Fatal("click passed through the notification into an action")
			}
			break
		}
	}
	if !clicked {
		t.Fatal("no action overlaps the notification")
	}
	var observed strings.Builder
	for range 12 {
		observed.WriteString(ansi.Strip(m.View().Content))
		card = m.noticeLayer
		m.Update(tea.MouseWheelMsg{X: card.x + 3, Y: card.y + 2, Button: tea.MouseWheelDown})
	}
	for i := range 24 {
		if !strings.Contains(observed.String(), fmt.Sprintf("DETAIL-%02d", i)) {
			t.Fatalf("scrolling lost notification line %d", i)
		}
	}
	m.View()
	card = m.noticeLayer
	m.Update(tea.MouseClickMsg{X: card.x + card.width - 3, Y: card.y + 1, Button: tea.MouseLeft})
	if screen := ansi.Strip(m.View().Content); strings.Contains(screen, "DETAIL-") || m.mode != "" {
		t.Fatal("closing the notification changed the page or left the card visible")
	}
	if m.status != strings.TrimSpace(message.String()) {
		t.Fatal("dismissal discarded the full notification message")
	}
}

func TestNotificationProgressPreservesOutcome(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.dismissNotification()
	m.progressStatus("loading")
	if m.notificationVisible() {
		t.Fatal("loading created a notification")
	}
	m.warnStatus("failed outcome")
	m.progressStatus("connecting")
	if !m.notificationVisible() || !m.notification.warning {
		t.Fatal("progress replaced the warning outcome")
	}
	lines, _, _ := m.notificationRows(54, 12)
	if strings.Join(lines, "\n") != "failed outcome" || m.status != "connecting" {
		t.Fatal("notification and bottom status did not remain independent")
	}
}

func TestNotificationExpiryDoesNotDismissReplacement(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.setStatus("first outcome")
	// 将首条通知的计时起点移到过去，无需等待真实的五秒。
	m.notification.started = time.Now().Add(-notificationDuration)
	oldTimer := m.notificationCommand()
	if oldTimer == nil {
		t.Fatal("notification did not schedule its expiry")
	}
	m.warnStatus("new warning")
	m.busy, m.obsBusy = true, true
	m.updateNotification(oldTimer().(notificationExpired))
	if !m.notificationVisible() {
		t.Fatal("stale expiry dismissed the newer warning")
	}
	m.notification.started = time.Now().Add(-notificationDuration)
	expiry := m.notificationCommand()
	if expiry == nil {
		t.Fatal("warning did not schedule its expiry")
	}
	if m.notificationCommand() != nil {
		t.Fatal("notification scheduled duplicate expiry")
	}
	m.updateNotification(expiry().(notificationExpired))
	if m.notificationVisible() || !m.notification.dismissed || m.status != "new warning" {
		t.Fatal("busy warning did not expire while preserving full status")
	}
	m.setStatus("info outcome")
	m.notification.started = time.Now().Add(-5 * time.Second)
	if m.notificationVisible() {
		t.Fatal("busy info notification lasted longer than five seconds")
	}
}

func TestNotificationWarningsOnlyKeepsFullStatus(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.config.TUINotificationsWarningsOnly = true
	m.setStatus("successful outcome")
	if m.notificationVisible() || m.status != "successful outcome" {
		t.Fatal("warnings-only did not suppress info while retaining status")
	}
	m.warnStatus("warning outcome")
	if !m.notificationVisible() {
		t.Fatal("warnings-only suppressed a warning")
	}
	m.notification.started = time.Now().Add(-5 * time.Second)
	if m.notificationVisible() {
		t.Fatal("warning lasted longer than five seconds")
	}
}

func TestDecorationPausesOnBlurAndResumesWithoutStaleTicks(t *testing.T) {
	m := lifecycleModel(t, context.Background())
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 36})
	m.dismissNotification()
	m.Init()
	initial := m.View().Content
	old := showcaseTick{generation: m.showcase.generation}
	for range 45 {
		m.Update(old)
	}
	animated := m.View().Content
	if animated == initial || ansi.Strip(animated) != ansi.Strip(initial) {
		t.Fatal("decoration must change color without moving visible content")
	}
	if full := m.View().Content; composeLayer(animated) != composeLayer(full) {
		t.Fatal("partial animation changed cells outside the separator")
	}
	m.Update(tea.BlurMsg{})
	frozen := m.View().Content
	if _, next := m.Update(old); next != nil {
		t.Fatal("background decoration scheduled another frame")
	}
	if got := m.View().Content; got != frozen {
		t.Fatal("decoration continued after focus was lost")
	}
	if _, next := m.Update(tea.FocusMsg{}); next == nil {
		t.Fatal("foreground decoration did not resume")
	}
	m.View()
	m.Update(old)
	if got := m.View().Content; got != frozen {
		t.Fatal("a pre-blur timer changed the resumed scene")
	}
	for range 45 {
		m.Update(showcaseTick{generation: m.showcase.generation})
	}
	if got := m.View().Content; got == frozen {
		t.Fatal("resumed decoration did not advance")
	}
}

func TestBackgroundPollingStillLoadsChat(t *testing.T) {
	m := chatTestModel(t)
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 36})
	m.dismissNotification()
	m.Init()
	m.View()
	m.Update(tea.BlurMsg{})
	before := m.View().Content
	appendChat(t, m, "background-message")
	_, cmd := m.Update(chatTick{})
	pending := []tea.Cmd{cmd}
	for len(pending) != 0 {
		next := pending[0]
		pending = pending[1:]
		if next == nil {
			continue
		}
		switch msg := next().(type) {
		case tea.BatchMsg:
			pending = append(pending, msg...)
		case chatPageMsg:
			m.Update(msg)
		}
	}
	after := m.View().Content
	if !strings.Contains(after, "background-message") {
		t.Fatal("background polling did not display the received message")
	}
	if strings.Split(before, "\n")[5] != strings.Split(after, "\n")[5] {
		t.Fatal("business polling restarted the background decoration")
	}
	m.Update(chatTick{})
	if got := m.View().Content; got != after {
		t.Fatal("idle polling changed the displayed chat")
	}
}
