package tui

import "arcana-world/internal/danmaku"

// 零宽标记随名字一起换行，记录实际文字边界，输出前移除。
const chatSpeakerBoundary = "\x1b]1337;arcana-speaker-end\a"

type chatSpeakerSpan struct {
	event              danmaku.Event
	row, column, width int
}

func (m *Model) chatSpeakerAt(x, y int) *danmaku.Event {
	if m.chat == nil || m.busy || m.previewing || m.page != chatPage || (m.mode != "" && m.mode != "chat-history") || m.pointerOverNotice() {
		return nil
	}
	spans := m.chat.speakerSpans
	if m.mode == "chat-history" && m.chat.historyBrowser != nil {
		spans = m.chat.historyBrowser.speakerSpans
	}
	column, row := x-m.pickerLeft, y-m.pickerTop+m.view.YOffset()
	for _, span := range spans {
		if row == span.row && column >= span.column && column < span.column+span.width {
			event := span.event
			return &event
		}
	}
	return nil
}
