package tui

import (
	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
)

// 退出确认保留编辑现场；后台结果暂存到取消确认后按顺序处理。
type exitConfirmation struct {
	mode, editKind, prompt, action string
	selected, offset               int
	pending                        []tea.Msg
}

func (m *Model) quitCommand() tea.Cmd {
	if clear := m.clearInlineImage(); clear != nil {
		return tea.Sequence(clear, tea.Quit)
	}
	return tea.Quit
}

func (m *Model) requestQuit() tea.Cmd {
	if m.exitPrompt != nil {
		return nil
	}
	m.exitPrompt = &exitConfirmation{mode: m.mode, editKind: m.editKind, prompt: m.prompt, action: m.confirmAction, selected: m.selected, offset: m.view.YOffset()}
	return m.confirm(i18n.T(i18n.TUIConfirmQuit), "quit")
}

func (m *Model) quitKey(key tea.KeyPressMsg) tea.Cmd {
	switch key.String() {
	case "left", "right", "up", "down", "tab", "h", "l", "j", "k":
		m.selected = 1 - m.selected
	case "esc":
		return m.cancelQuit()
	case "enter":
		if m.selected == 0 {
			return m.cancelQuit()
		}
		m.session.BeginClose()
		if m.cancel != nil {
			m.cancel()
		}
		if m.obsCancel != nil {
			m.obsCancel()
		}
		return m.quitCommand()
	}
	return nil
}

func (m *Model) cancelQuit() tea.Cmd {
	state := m.exitPrompt
	m.exitPrompt = nil
	m.mode, m.editKind, m.prompt, m.confirmAction = state.mode, state.editKind, state.prompt, state.action
	m.selected = state.selected
	m.syncWorkspace()
	m.view.SetContent(m.content())
	m.view.SetYOffset(state.offset)
	commands := make([]tea.Cmd, 0, len(state.pending))
	for _, msg := range state.pending {
		_, cmd := m.Update(msg)
		commands = append(commands, cmd)
	}
	return tea.Batch(commands...)
}

func (m *Model) deferUntilQuitResolved(msg tea.Msg) bool {
	if m.exitPrompt == nil {
		return false
	}
	switch msg.(type) {
	case tea.KeyPressMsg, tea.KeyReleaseMsg, tea.PasteMsg, tea.PasteStartMsg, tea.PasteEndMsg, tea.MouseMsg, tea.WindowSizeMsg, tea.BackgroundColorMsg, notificationExpired, uv.CellSizeEvent, uv.KittyGraphicsEvent, imageTimeout:
		return false
	default:
		m.exitPrompt.pending = append(m.exitPrompt.pending, msg)
		return true
	}
}
