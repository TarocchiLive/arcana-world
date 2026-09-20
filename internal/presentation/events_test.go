package presentation

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"arcana-world/internal/danmaku"
)

func TestTemplatesUseProjectedNestedFields(t *testing.T) {
	history, err := danmaku.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer history.Close()
	_, err = history.Append(1, []byte(`{"cmd":"LIVE_OPEN_PLATFORM_GUARD","data":{"user_info":{"uname":"测试观众"},"guard_level":3,"guard_num":2}}`))
	if err != nil {
		t.Fatal(err)
	}
	events, err := history.Page(1, 0, 1)
	if err != nil || len(events) != 1 {
		t.Fatalf("projected event: %v %v", events, err)
	}
	for _, render := range []func(danmaku.Event) string{RenderOverlay, RenderTTS} {
		text := render(events[0])
		for _, value := range []string{"测试观众", "舰长", "2"} {
			if !strings.Contains(text, value) {
				t.Fatalf("lost projected value %q: %q", value, text)
			}
		}
		if strings.Contains(text, "user_info") || strings.Contains(text, "guard_level") {
			t.Fatalf("protocol fields exposed: %q", text)
		}
	}
}

func TestOutputCannotLeakDeletedTextOrTerminalControls(t *testing.T) {
	event := danmaku.Event{Kind: "chat", User: "\x1b[31m观众\x1b[0m\u202e", Text: strings.Repeat("界", 2000) + "\x1b]0;injected-title\a"}
	spoken := RenderTTS(event)
	if !utf8.ValidString(spoken) || utf8.RuneCountInString(spoken) > 500 || !strings.Contains(spoken, "观众") {
		t.Fatalf("invalid speech bounds: %q", spoken)
	}
	for _, render := range []func(danmaku.Event) string{RenderOverlay, RenderTTS} {
		text := render(event)
		if strings.Contains(text, "injected-title") || strings.ContainsFunc(text, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) {
			t.Fatalf("unsafe output: %q", text)
		}
		deleted := event
		deleted.Deleted = true
		deleted.Text = "deleted-private-message"
		if strings.Contains(render(deleted), deleted.Text) {
			t.Fatal("deleted message leaked")
		}
	}
	hidden := danmaku.Event{Kind: "detail", Text: "ENTRY_EFFECT"}
	if Enabled(nil, hidden) || RenderOverlay(hidden) != "" || RenderTTS(hidden) != "" {
		t.Fatal("additional event escaped default event selection")
	}
}

func TestGuardDurationPreservesAnnualAndExplicitUnits(t *testing.T) {
	for _, test := range []struct {
		name, raw, duration string
	}{
		{"annual", `{"cmd":"USER_TOAST_MSG_V2","data":{"sender_uinfo":{"uid":42,"base":{"name":"观众"}},"guard_info":{"guard_level":2},"pay_info":{"num":1,"unit":"年"}}}`, "1年"},
		{"explicit duration", `{"cmd":"LIVE_OPEN_PLATFORM_GUARD","data":{"user_info":{"uname":"观众"},"guard_level":3,"guard_num":99,"guard_unit":"*3天"}}`, "3天"},
	} {
		t.Run(test.name, func(t *testing.T) {
			history, err := danmaku.Open(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer history.Close()
			if _, err := history.Append(1, []byte(test.raw)); err != nil {
				t.Fatal(err)
			}
			events, err := history.Page(1, 0, 1)
			if err != nil || len(events) != 1 {
				t.Fatalf("read purchase: %v %v", events, err)
			}
			for _, render := range []func(danmaku.Event) string{RenderOverlay, RenderTTS} {
				text := render(events[0])
				if !strings.Contains(text, test.duration) || strings.Contains(text, "个月") || strings.Contains(text, "99") {
					t.Fatalf("subscription duration misrepresented: %q", text)
				}
			}
		})
	}
}
