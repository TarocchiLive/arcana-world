package danmaku

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"math/rand/v2"
	"sync"
	"time"

	"arcana-world/internal/bili"
	"arcana-world/internal/domain"
)

const (
	roomLookupTimeout    = 30 * time.Second
	initialRetryDelay    = time.Second
	maxReconnectDelay    = 30 * time.Second
	maxBackoffExponent   = 5
	reconnectJitterParts = 4
	stableSessionPeriod  = time.Minute
)

// Snapshot contains status only: message delivery never depends on the UI.
type Snapshot struct {
	Phase    string
	RoomID   int64
	Revision uint64
	Err      error
	// AccountUID and Generation identify the desired source even while the
	// previous session is stopping. RoomID is zero until this source publishes.
	AccountUID string
	Generation uint64
}
type pendingWrite struct {
	room  int64
	write func() (bool, error)
}

type source interface {
	Room(context.Context) (domain.Room, error)
	ListenDanmaku(context.Context, int64, int, func(), func(json.RawMessage) error) error
}
type archive interface {
	Append(int64, json.RawMessage) (bool, error)
	RecordGap(int64, string) error
}
type target struct {
	account domain.Account
	proxy   string
	enabled bool
}

// Listener serializes target changes; at most one session writes to history.
// Settings notifications may coalesce. Business messages never use a lossy channel.
type Listener struct {
	mu              sync.Mutex
	desired         target
	generation      uint64
	stateGeneration uint64
	state           Snapshot
	history         archive
	factory         func(string, domain.Account) (source, error)
	pending         []pendingWrite // bounded by the last decoded frame; drained before any new session
	wake            chan struct{}
	cancel          context.CancelFunc
	done            chan struct{}
	closeOnce       sync.Once
	closeErr        error
	retryDelay      time.Duration
}

func NewListener(ctx context.Context, history *History) *Listener {
	return newListener(ctx, history, func(proxy string, account domain.Account) (source, error) {
		c, err := bili.New(proxy)
		if err != nil {
			return nil, err
		}
		c.SetAccount(account)
		return c, nil
	})
}

func newListener(ctx context.Context, history archive, factory func(string, domain.Account) (source, error)) *Listener {
	ctx, cancel := context.WithCancel(ctx)
	l := &Listener{history: history, factory: factory, wake: make(chan struct{}, 1), cancel: cancel, done: make(chan struct{}), retryDelay: initialRetryDelay, state: Snapshot{Phase: "waiting"}}
	go l.control(ctx)
	return l
}

// Configure is idempotent, including credentials; it never waits on network I/O.
func (l *Listener) Configure(account *domain.Account, proxy string, enabled bool) {
	next := target{proxy: proxy, enabled: enabled}
	if account != nil {
		next.account = *account
		next.account.Cookies = maps.Clone(account.Cookies)
	}
	l.mu.Lock()
	old := l.desired
	if l.generation != 0 && old.enabled == next.enabled && old.proxy == next.proxy && old.account.UID == next.account.UID && maps.Equal(old.account.Cookies, next.account.Cookies) {
		l.mu.Unlock()
		return
	}
	l.desired = next
	l.generation++
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// Retry explicitly renews a failed session without changing persisted settings.
func (l *Listener) Retry() {
	l.mu.Lock()
	l.generation++
	l.mu.Unlock()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *Listener) Snapshot() Snapshot {
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.state
	state.AccountUID, state.Generation = l.desired.account.UID, l.generation
	if l.stateGeneration != l.generation {
		state.RoomID = 0
		if state.Phase != "storage_error" {
			state.Phase, state.Err = "waiting", nil
		}
	}
	return state
}
func (l *Listener) Close() error {
	l.closeOnce.Do(func() { l.cancel(); <-l.done })
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.closeErr
}
func (l *Listener) publish(generation uint64, phase string, room int64, err error, changed bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if generation != l.generation && phase != "storage_error" {
		return
	}
	l.state.Phase, l.state.RoomID, l.state.Err = phase, room, err
	l.stateGeneration = generation
	if changed {
		l.state.Revision++
	}
}
func (l *Listener) retainError(err error) {
	if err == nil {
		return
	}
	l.mu.Lock()
	l.closeErr = errors.Join(l.closeErr, err)
	l.mu.Unlock()
}

func (l *Listener) control(ctx context.Context) {
	defer close(l.done)
	var stop context.CancelFunc
	var finished chan struct{}
	var activeGeneration uint64
	stopSession := func() {
		if stop != nil {
			stop()
			<-finished
			stop = nil
		}
	}
	defer func() {
		stopSession()
		for _, p := range l.pending {
			_, err := p.write()
			l.retainError(err)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.wake:
			l.mu.Lock()
			gen := l.generation
			l.mu.Unlock()
			if gen == activeGeneration {
				continue
			}
			stopSession()
			// A target switch must not discard a message whose disk write failed.
			// Retain its closure and drain it before accepting events from another room.
			for len(l.pending) > 0 {
				p := l.pending[0]
				if err := l.persist(ctx, activeGeneration, p.room, p.write); err != nil {
					return
				}
				l.pending[0] = pendingWrite{}
				l.pending = l.pending[1:]
			}
			l.mu.Lock()
			next := l.desired
			gen = l.generation
			l.mu.Unlock()
			activeGeneration = gen
			if ctx.Err() != nil {
				return
			}
			if !next.enabled {
				l.publish(gen, "disabled", 0, nil, true)
				continue
			}
			if next.account.UID == "" {
				l.publish(gen, "waiting", 0, nil, true)
				continue
			}
			session, cancel := context.WithCancel(ctx)
			stop, finished = cancel, make(chan struct{})
			done := finished
			go func() { defer close(done); l.run(session, next, gen) }()
		}
	}
}

// persist retries the same payload before accepting another. On storage failure
// memory is bounded by the current wire frame and transport buffers, not an
// unbounded queue. Shutdown attempts the write once more and reports failure.
func (l *Listener) persist(ctx context.Context, gen uint64, room int64, write func() (bool, error)) error {
	phase := l.Snapshot().Phase
	if phase == "storage_error" {
		phase = "connecting"
	}
	for {
		changed, err := write()
		if err == nil {
			l.publish(gen, phase, room, nil, changed)
			return nil
		}
		l.publish(gen, "storage_error", room, err, false)
		if !sleep(ctx, l.retryDelay) {
			_, finalErr := write()
			return finalErr
		}
	}
}

func (l *Listener) save(ctx context.Context, gen uint64, room int64, write func() (bool, error)) error {
	if len(l.pending) > 0 {
		// Once cancellation has queued a failed write, preserve receive order
		// through the rest of the decoded frame even if the disk recovers.
		l.pending = append(l.pending, pendingWrite{room: room, write: write})
		return nil
	}
	err := l.persist(ctx, gen, room, write)
	if err != nil {
		l.pending = append(l.pending, pendingWrite{room: room, write: write})
	}
	// The canceled wire reader must finish decoding the frame it already read.
	// Pending writes stay in memory and are drained before another session; a
	// final shutdown failure is returned by Close, never counted as committed.
	return nil
}

func (l *Listener) run(ctx context.Context, next target, gen uint64) {
	c, err := l.factory(next.proxy, next.account)
	if err != nil {
		l.publish(gen, "error", 0, err, true)
		return
	}
	if client, ok := c.(*bili.Client); ok {
		defer client.HTTP.CloseIdleConnections()
	}
	var room int64
	attempt, failures := 0, 0
	for ctx.Err() == nil {
		l.publish(gen, "connecting", room, nil, false)
		if room == 0 {
			callCtx, cancel := context.WithTimeout(ctx, roomLookupTimeout)
			info, roomErr := c.Room(callCtx)
			cancel()
			err = roomErr
			if err == nil {
				room = info.ID
			}
			if err == nil && room <= 0 {
				err = errors.New("invalid live room")
			}
		} else {
			err = nil
		}
		if err == nil {
			// Every session boundary is durable, including clean restart. There is no
			// server replay cursor, so a reconnect must never imply complete coverage.
			err = l.save(ctx, gen, room, func() (bool, error) { return true, l.history.RecordGap(room, "session_start") })
			if err != nil || ctx.Err() != nil {
				return
			}
			l.publish(gen, "connecting", room, nil, false)
			started := time.Now()
			authenticated := false
			err = c.ListenDanmaku(ctx, room, attempt, func() {
				authenticated = true
				l.publish(gen, "connected", room, nil, false)
			}, func(raw json.RawMessage) error {
				return l.save(ctx, gen, room, func() (bool, error) { return l.history.Append(room, raw) })
			})
			// Do not reset backoff merely on auth: repeatedly dying sockets otherwise
			// cause a reconnect storm.
			if authenticated && time.Since(started) >= stableSessionPeriod {
				failures = 0
			}
			gap := "connection_lost"
			if ctx.Err() != nil {
				gap = "session_end"
			}
			gapErr := l.save(ctx, gen, room, func() (bool, error) { return true, l.history.RecordGap(room, gap) })
			if gapErr != nil {
				return
			}
		}
		if ctx.Err() != nil {
			return
		}
		l.publish(gen, "reconnecting", room, err, true)
		delay := min(maxReconnectDelay, l.retryDelay*time.Duration(1<<min(failures, maxBackoffExponent)))
		delay += time.Duration(rand.Int64N(max(1, int64(delay/reconnectJitterParts))))
		failures++
		attempt++
		if !sleep(ctx, delay) {
			return
		}
	}
}

func sleep(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
