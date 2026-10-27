package tui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
)

func (r taskResult[T]) apply(m *Model) tea.Cmd {
	r.logOutcome(m)
	if m.canceled && r.operation.discardOnCancel {
		m.log(i18n.T(i18n.TUIStatusOperationCanceled))
		return nil
	}
	if r.err == nil {
		m.config = m.store.Config()
	}
	return r.operation.handle(m, r.value, r.err, r.operation.label)
}

// logOutcome 在操作边界记录结果：每个失败的操作恰好产生一条 ERROR。
func (r taskResult[T]) logOutcome(m *Model) {
	var elapsed time.Duration
	if !r.started.IsZero() {
		elapsed = time.Since(r.started)
	}
	op := string(r.operation.label)
	var face *domain.FaceChallenge
	switch {
	case r.err == nil && m.canceled && r.operation.discardOnCancel:
		logger.Info("operation result discarded after cancel", "op", op, "id", r.id, "elapsed", elapsed)
	case r.err == nil && r.operation.periodic:
		logger.Debug("operation succeeded", "op", op, "id", r.id, "elapsed", elapsed)
	case r.err == nil:
		logger.Info("operation succeeded", "op", op, "id", r.id, "elapsed", elapsed)
	case errors.As(r.err, &face) && !m.canceled:
		logger.Info("face challenge required", "op", op, "id", r.id, "elapsed", elapsed, "err", r.err)
	case errors.Is(r.err, context.Canceled):
		logger.Warn("operation canceled", "op", op, "id", r.id, "elapsed", elapsed, "user_canceled", m.canceled, "err", r.err)
	default:
		logger.Error("operation failed", "op", op, "id", r.id, "elapsed", elapsed, "err", r.err)
	}
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
	if m.web.enabled {
		return nil
	}
	if m.settingsDetail != "" && (m.mode != "pick" || m.editKind != "settings-detail") {
		return m.showSettingsDetail()
	}
	m.view.SetContent(m.content())
	if m.page == settingsPage && m.mode == "" {
		m.view.SetYOffset(m.settingsPageOffset)
	}
	return nil
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
