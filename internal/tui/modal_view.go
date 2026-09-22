package tui

import (
	"fmt"
	"strings"

	"arcana-world/internal/i18n"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func (m *Model) modalContent() string {
	width := max(1, m.view.Width())
	switch m.mode {
	case "chat-history-range":
		return m.chatHistoryRangeView()
	case "chat-history":
		return m.chatHistoryView()
	case "help":
		paragraphs := strings.Split(strings.TrimSpace(i18n.T(i18n.Key(m.editKind))), "\n\n")
		rows := make([]string, 0, len(paragraphs)+1)
		for i, paragraph := range paragraphs {
			if i == 0 {
				rows = append(rows, m.theme.sectionTitle(paragraph, width))
			} else {
				heading, body, separated := strings.Cut(paragraph, "\n")
				if separated && strings.HasPrefix(body, "https://") && !strings.ContainsAny(body, " \n\t") {
					body = ansi.SetHyperlink(body) + body + ansi.ResetHyperlink()
				}
				if separated {
					rows = append(rows, lipgloss.JoinVertical(lipgloss.Left,
						m.theme.accent.Render(ansi.Wrap(heading, width, "")),
						m.theme.textStyle.Render(ansi.Wrap(body, width, ""))))
				} else {
					rows = append(rows, m.theme.textStyle.Render(ansi.Wrap(paragraph, width, "")))
				}
			}
		}
		rows = append(rows, m.theme.hintText(i18n.T(i18n.TUIHelpArticleControls), width))
		return strings.Join(rows, "\n\n")
	case "cover", "cover-review":
		return m.coverView()
	case "selection":
		return m.selection.View(width, m.view.Height())
	case "form":
		controls := i18n.T(i18n.TUIFormControls)
		if m.editKind == "cover-path" {
			controls = i18n.T(i18n.TUIFormCoverControls)
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			m.formPrompt(),
			m.input.View()+m.overlayColorSample(),
			lipgloss.NewStyle().PaddingTop(1).Render(m.theme.hintText(strings.TrimSpace(controls), width)))
	case "confirm":
		cancel := m.theme.listRow(strings.TrimSpace(i18n.T(i18n.TUIConfirmCancelLabel)), width, m.selected == 0)
		executeLabel := i18n.TUIConfirmExecuteLabel
		if m.exitPrompt != nil {
			executeLabel = i18n.TUIConfirmQuitAction
		}
		execute := m.theme.listRow(strings.TrimSpace(i18n.T(executeLabel)), width, m.selected == 1)
		if m.selected == 1 {
			execute = m.theme.selectedStyle.Foreground(m.theme.danger.GetForeground()).Render(ansi.Strip(execute))
		} else {
			execute = m.theme.danger.Render(ansi.Strip(execute))
		}
		return lipgloss.JoinVertical(lipgloss.Left,
			m.theme.warning.Width(width).PaddingBottom(1).Render(clean(m.prompt)),
			cancel, execute,
			lipgloss.NewStyle().PaddingTop(1).Render(m.theme.hintText(i18n.T(i18n.TUIConfirmControls), width)))
	case "pick":
		// 鼠标命中检测要求首个选项之前恰好保留两行。
		rows := []string{lipgloss.NewStyle().PaddingBottom(1).Render(m.theme.sectionTitle(selectionText(m.prompt), width))}
		window := m.pickerWindow()
		start := max(0, m.selected-window+1)
		end := min(len(m.choices), start+window)
		for i := start; i < end; i++ {
			text := m.choices[i].label
			if m.editKind == "output-events" {
				text = m.outputEventLabel(m.choices[i])
			} else if m.editKind == "overlay-displays" {
				text = m.overlayDisplayLabel(m.choices[i])
			}
			rows = append(rows, m.theme.listRow(text, width, i == m.selected))
		}
		controls := i18n.TUISelectionPickerControls
		if m.editKind == "output-events" {
			controls = i18n.OutputEventsControls
		} else if m.editKind == "overlay-displays" {
			controls = i18n.OutputDisplaysControls
		}
		position := 0
		if len(m.choices) > 0 {
			position = max(1, min(m.selected+1, len(m.choices)))
		}
		rows = append(rows, lipgloss.NewStyle().PaddingTop(1).
			Render(m.theme.hintText(strings.TrimSpace(fmt.Sprintf(i18n.T(controls), position, len(m.choices))), width)))
		if ansi.StringWidth(selectionText(m.prompt)) > width || strings.Contains(m.prompt, "\n") {
			rows = append(rows, lipgloss.NewStyle().PaddingTop(1).Render(m.theme.hintText(m.prompt, width)))
		}
		return lipgloss.JoinVertical(lipgloss.Left, rows...)
	case "qr", "face":
		title := i18n.T(i18n.TUIQRSignInTitle)
		link := ""
		if m.qr != nil {
			link = m.qr.URL
		}
		if m.mode == "face" {
			title = i18n.T(i18n.TUIQRIdentityTitle)
			link = m.faceURL
		}
		// 保持二维码位图原有颜色，确保扫描可靠。
		return lipgloss.JoinVertical(lipgloss.Left,
			m.theme.sectionTitle(title, width),
			lipgloss.NewStyle().PaddingBottom(1).Render(m.theme.hintText(i18n.T(i18n.TUIQRVisibilityHint), width)),
			m.theme.warning.Width(width).PaddingBottom(1).Render(clean(i18n.T(i18n.TUIQRTokenWarning))),
			m.qrText,
			m.theme.textStyle.Width(width).Render(clean(link)))
	case "login-save":
		return lipgloss.JoinVertical(lipgloss.Left,
			m.theme.warning.Width(width).PaddingBottom(1).Render(clean(i18n.T(i18n.TUILoginUnsavedTitle))),
			m.theme.hintText(strings.TrimSpace(i18n.T(i18n.TUILoginKeyringRetry)), width))
	}
	return ""
}

func (m *Model) formPrompt() string {
	style := m.theme.accent
	if m.editKind == "clear-data" {
		style = m.theme.danger
	}
	return style.Width(max(1, m.view.Width())).PaddingBottom(1).Render(clean(m.prompt))
}
