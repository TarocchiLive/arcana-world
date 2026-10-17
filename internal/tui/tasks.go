package tui

import (
	"arcana-world/internal/i18n"
	tea "charm.land/bubbletea/v2"
	"context"
	"fmt"
	"time"
)

// 普通任务共用操作编号；OBS 与浮层生命周期任务独立处理。
type taskMessage interface {
	taskID() int
	taskError() error
	apply(*Model) tea.Cmd
}
type operation[T any] struct {
	label           i18n.Key
	discardOnCancel bool
	periodic        bool // 周期性轮询：成功只记 DEBUG，避免默认级别下刷屏
	handle          func(*Model, T, error, i18n.Key) tea.Cmd
}
type taskResult[T any] struct {
	id        int
	operation operation[T]
	value     T
	err       error
	started   time.Time // 任务开始执行的时间，用于计算耗时
}

// start 记录操作开始；仅 DEBUG，成功或失败由 apply 在操作边界统一记录。
func (o operation[T]) start(id int) {
	logger.Debug("operation started", "op", string(o.label), "id", id)
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
	m.progressStatus(fmt.Sprintf(i18n.T(i18n.TUIStatusOperationPending), i18n.T(op.label)))
	op.start(id)
	return func() tea.Msg {
		defer cancel()
		started := time.Now()
		value, err := fn(ctx)
		return taskResult[T]{id: id, operation: op, value: value, err: err, started: started}
	}
}
