package tts

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type stopResult struct {
	done chan struct{}
	err  error
}

type Manager struct {
	mu          sync.Mutex
	ctx         context.Context
	synth       Synthesizer
	player      Player
	queue       []string
	head, count int
	state       Snapshot
	cancel      context.CancelFunc
	stopping    *stopResult
	closeErr    error
	wake        chan struct{}
	done        chan struct{}
}

func New(ctx context.Context, options Options) (*Manager, error) {
	if ctx == nil || options.Synthesizer == nil || options.Player == nil || options.QueueCapacity < 0 {
		return nil, errors.New("tts: invalid options")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	capacity := options.QueueCapacity
	if capacity == 0 {
		capacity = DefaultQueueCapacity
	}
	m := &Manager{ctx: ctx, synth: options.Synthesizer, player: options.Player, queue: make([]string, capacity), state: Snapshot{Phase: PhaseIdle}, wake: make(chan struct{}, 1), done: make(chan struct{})}
	// AfterFunc 避免额外创建生命周期协程，且绝不在持锁时等待。
	stop := context.AfterFunc(ctx, func() { m.Close() })
	go func() { defer stop(); defer close(m.done); m.run() }()
	return m, nil
}
func (m *Manager) notify() {
	select {
	case m.wake <- struct{}{}:
	default:
	}
}
func (m *Manager) Enqueue(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ctx.Err() != nil {
		m.closeLocked()
	}
	if m.state.Phase == PhaseClosed {
		return ErrClosed
	}
	if m.state.Phase == PhaseStopping {
		return ErrStopping
	}
	text, err := validateText(text)
	if err != nil {
		return err
	}
	if m.count == len(m.queue) {
		m.state.Dropped++
		return ErrQueueFull
	}
	m.queue[(m.head+m.count)%len(m.queue)] = text
	m.count++
	m.notify()
	return nil
}
func (m *Manager) clear() { clear(m.queue); m.head = 0; m.count = 0 }
func (m *Manager) Stop() error {
	m.mu.Lock()
	if m.ctx.Err() != nil {
		m.closeLocked()
	}
	if m.state.Phase == PhaseClosed {
		m.mu.Unlock()
		<-m.done
		return m.closeErr
	}
	if m.state.Phase == PhaseStopping {
		result := m.stopping
		m.mu.Unlock()
		<-result.done
		return result.err
	}
	m.clear()
	if m.cancel == nil {
		m.mu.Unlock()
		return nil
	}
	m.state.Phase = PhaseStopping
	result := &stopResult{done: make(chan struct{})}
	m.stopping = result
	m.cancel()
	m.mu.Unlock()
	<-result.done
	return result.err
}
func (m *Manager) Close() error {
	m.mu.Lock()
	m.closeLocked()
	m.mu.Unlock()
	<-m.done
	return m.closeErr
}

func (m *Manager) closeLocked() {
	m.state.Phase = PhaseClosed
	m.clear()
	if m.cancel != nil {
		m.cancel()
	}
	m.notify()
}
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.state
	s.Pending = m.count
	return s
}
func (m *Manager) Done() <-chan struct{} { return m.done }
func (m *Manager) run() {
	for {
		m.mu.Lock()
		if m.ctx.Err() != nil {
			m.closeLocked()
		}
		if m.state.Phase == PhaseClosed {
			m.mu.Unlock()
			return
		}
		if m.count == 0 {
			m.mu.Unlock()
			<-m.wake
			continue
		}
		text := m.queue[m.head]
		m.queue[m.head] = ""
		m.head = (m.head + 1) % len(m.queue)
		m.count--
		ctx, cancel := context.WithCancel(m.ctx)
		m.cancel = cancel
		m.state.Phase = PhaseSynthesizing
		m.mu.Unlock()
		audio, err := m.synth.Synthesize(ctx, text)
		if err != nil {
			err = fmt.Errorf("tts: synthesize: %w", err)
		}
		m.mu.Lock()
		play := err == nil && ctx.Err() == nil && m.state.Phase != PhaseClosed && m.state.Phase != PhaseStopping
		if play {
			m.state.Phase = PhasePlaying
		}
		m.mu.Unlock()
		if play {
			if e := m.player.Play(ctx, audio); e != nil {
				err = fmt.Errorf("tts: play: %w", e)
			}
		}
		m.mu.Lock()
		if m.ctx.Err() != nil {
			m.closeLocked()
		}
		if err != nil && !cancellationOnly(err, ctx.Err()) {
			m.state.LastError = err
			if m.stopping != nil {
				m.stopping.err = err
			}
			if m.state.Phase == PhaseClosed {
				m.closeErr = err
			}
		}
		cancel()
		m.cancel = nil
		if m.stopping != nil {
			close(m.stopping.done)
			m.stopping = nil
		}
		if m.state.Phase != PhaseClosed {
			m.state.Phase = PhaseIdle
		}
		m.mu.Unlock()
	}
}

// 即使同时发生取消，合并后的清理失败也必须保留。
func cancellationOnly(err, cancellation error) bool {
	if cancellation == nil {
		return false
	}
	switch wrapped := err.(type) {
	case interface{ Unwrap() []error }:
		for _, child := range wrapped.Unwrap() {
			if !cancellationOnly(child, cancellation) {
				return false
			}
		}
		return errors.Is(err, cancellation)
	case interface{ Unwrap() error }:
		return cancellationOnly(wrapped.Unwrap(), cancellation)
	default:
		return errors.Is(err, cancellation)
	}
}
