package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestChatInteractionVisibilityFollowsActionAndOtherToggle(t *testing.T) {
	m := chatTestModel(t)
	for action := 1; action <= 5; action++ {
		raw := json.RawMessage(fmt.Sprintf(`{"cmd":"INTERACT_WORD","data":{"uid":1,"uname":"interaction-user-%d","msg_type":%d}}`, action, action))
		if _, err := m.chat.history.Append(1, raw); err != nil {
			t.Fatal(err)
		}
	}
	runChatCommand(m, m.readChat())
	for _, other := range []bool{false, true, false} {
		m.config.DanmakuShowOther = other
		runChatCommand(m, m.readChat())
		rendered := m.chatView(m.view.Width())
		for action := 1; action <= 5; action++ {
			want := action <= 2 || (action == 3 || action == 4) && other
			if got := strings.Contains(rendered, fmt.Sprintf("interaction-user-%d", action)); got != want {
				t.Fatalf("action=%d Other=%v: visible=%v want=%v", action, other, got, want)
			}
		}
	}
}
