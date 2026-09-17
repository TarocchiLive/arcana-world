package tui

import (
	"context"
	"errors"
	"fmt"

	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	tea "github.com/charmbracelet/bubbletea"
)

func (r taskResult[T]) apply(m *Model) tea.Cmd {
	if m.canceled && r.operation.discardOnCancel {
		m.log(i18n.T(i18n.TUIStatusOperationCanceled))
		return nil
	}
	if r.err == nil {
		m.config = m.store.Config()
	}
	return r.operation.handle(m, r.value, r.err)
}
func (m *Model) resultError(name string, err error) (tea.Cmd, bool) {
	if err == nil {
		return nil, false
	}
	var face *domain.FaceChallenge
	if errors.As(err, &face) && !m.canceled {
		return work(m, faceOperation(), func(ctx context.Context) (string, error) { return m.client.ResolveFace(ctx, face) }), true
	}
	if errors.Is(err, context.Canceled) {
		m.log(i18n.T(i18n.TUIStatusRemoteCancelWarning))
	} else {
		m.log(fmt.Sprintf(i18n.T(i18n.TUILogOperationFailed), operationName(name), err.Error()))
	}
	return nil, true
}
func (m *Model) finishResult() tea.Cmd {
	m.view.SetContent(m.content())
	return m.updateOverlayChat()
}

// Completion-only operations share error reporting and the normal success notice.
func completionOperation(name string) operation[struct{}] {
	return operation[struct{}]{
		name: name,
		handle: func(m *Model, _ struct{}, err error) tea.Cmd {
			if cmd, failed := m.resultError(name, err); failed {
				return cmd
			}
			m.log(fmt.Sprintf(i18n.T(i18n.TUILogOperationSucceeded), operationName(name)))
			return m.finishResult()
		},
	}
}
