package tui

import (
	"context"
	"errors"
	"fmt"

	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
)

func (r taskResult[T]) apply(m *Model) tea.Cmd {
	if m.canceled && r.operation.discardOnCancel {
		m.log(i18n.T(i18n.TUIStatusOperationCanceled))
		return nil
	}
	if r.err == nil {
		m.config = m.store.Config()
	}
	return r.operation.handle(m, r.value, r.err, r.operation.label)
}
func (m *Model) resultError(label i18n.Key, err error) (tea.Cmd, bool) {
	if err == nil {
		return nil, false
	}
	var face *domain.FaceChallenge
	if errors.As(err, &face) && !m.canceled {
		return work(m, faceOperation(), func(ctx context.Context) (string, error) { return m.client.ResolveFace(ctx, face) }), true
	}
	if errors.Is(err, context.Canceled) {
		m.warn(i18n.T(i18n.TUIStatusRemoteCancelWarning))
	} else {
		m.warn(fmt.Sprintf(i18n.T(i18n.TUILogOperationFailed), i18n.T(label), err.Error()))
	}
	return nil, true
}
func (m *Model) finishResult() tea.Cmd {
	m.view.SetContent(m.content())
	return m.updateOverlayChat()
}

// 仅返回完成状态的操作共用错误处理与成功提示。
func completionOperation(label i18n.Key) operation[struct{}] {
	return operation[struct{}]{
		label: label,
		handle: func(m *Model, _ struct{}, err error, label i18n.Key) tea.Cmd {
			return m.completeOperation(label, err)
		},
	}
}

func (m *Model) completeOperation(label i18n.Key, err error) tea.Cmd {
	if cmd, failed := m.resultError(label, err); failed {
		return cmd
	}
	m.log(fmt.Sprintf(i18n.T(i18n.TUILogOperationSucceeded), i18n.T(label)))
	return m.finishResult()
}
