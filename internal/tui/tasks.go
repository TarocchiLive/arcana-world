package tui

import (
	"arcana-world/internal/i18n"
	"context"
	"fmt"
	tea "github.com/charmbracelet/bubbletea"
)

// Ordinary tasks share an operation ID; OBS and overlay lifecycle tasks do not.
type taskMessage interface {
	taskID() int
	taskError() error
	apply(*Model) tea.Cmd
}
type operation[T any] struct {
	label           i18n.Key
	discardOnCancel bool
	handle          func(*Model, T, error, i18n.Key) tea.Cmd
}
type taskResult[T any] struct {
	id        int
	operation operation[T]
	value     T
	err       error
}

func (r taskResult[T]) taskID() int      { return r.id }
func (r taskResult[T]) taskError() error { return r.err }
func work[T any](m *Model, op operation[T], fn func(context.Context) (T, error)) tea.Cmd {
	if m.busy {
		return nil
	}
	m.busy = true
	m.canceled = false
	m.operation++
	id := m.operation
	ctx, cancel := context.WithTimeout(m.ctx, operationTimeout)
	m.cancel = cancel
	m.status = fmt.Sprintf(i18n.T(i18n.TUIStatusOperationPending), i18n.T(op.label))
	return func() tea.Msg {
		defer cancel()
		value, err := fn(ctx)
		return taskResult[T]{id: id, operation: op, value: value, err: err}
	}
}
