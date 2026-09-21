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

// Phase 标识监听器状态，不额外引入状态转换引擎。
// 字符串值保持稳定，供状态使用方依赖。
type Phase string

const (
	PhaseWaiting      Phase = "waiting"
	PhaseDisabled     Phase = "disabled"
	PhaseConnecting   Phase = "connecting"
	PhaseConnected    Phase = "connected"
	PhaseReconnecting Phase = "reconnecting"
	PhaseStorageError Phase = "storage_error"
	PhaseError        Phase = "error"
)

// Snapshot 仅包含状态：消息交付绝不依赖 UI。
type Snapshot struct {
	Phase    Phase
	RoomID   int64
	Revision uint64
	Err      error
	// AccountUID 和 Generation 标识期望的数据源，即使
	// 上一个会话仍在停止。此数据源发布状态前，RoomID 为零。
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

// Listener 串行处理目标变更；最多只有一个会话写入历史记录。
// 设置通知可以合并。业务消息绝不使用会丢数据的通道。
type Listener struct {
	mu              sync.Mutex
	desired         target
	generation      uint64
	stateGeneration uint64
	state           Snapshot
	history         archive
	factory         func(string, domain.Account) (source, error)
	pending         []pendingWrite // 上限为最后一个已解码帧的内容；任何新会话开始前必须写完
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
	l := &Listener{history: history, factory: factory, wake: make(chan struct{}, 1), cancel: cancel, done: make(chan struct{}), retryDelay: initialRetryDelay, state: Snapshot{Phase: PhaseWaiting}}
	go l.control(ctx)
	return l
}

// Configure 包括凭据配置在内均为幂等操作；绝不等待网络 I/O。
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

// Retry 显式重建失败的会话，不修改持久化设置。
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
		if state.Phase != PhaseStorageError {
			state.Phase, state.Err = PhaseWaiting, nil
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
func (l *Listener) publish(generation uint64, phase Phase, room int64, err error, changed bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if generation != l.generation && phase != PhaseStorageError {
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
			// 切换目标不得丢弃写盘失败的消息。
			// 保留其闭包，在接收其他直播间的事件前完成所有待写入操作。
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
				l.publish(gen, PhaseDisabled, 0, nil, true)
				continue
			}
			if next.account.UID == "" {
				l.publish(gen, PhaseWaiting, 0, nil, true)
				continue
			}
			session, cancel := context.WithCancel(ctx)
			stop, finished = cancel, make(chan struct{})
			done := finished
			go func() { defer close(done); l.run(session, next, gen) }()
		}
	}
}

// persist 在接受下一份载荷前重试当前载荷。存储失败时，
// 内存用量受当前线格式帧和传输缓冲区限制，
// 而非使用无界队列。关闭时会再尝试写入一次，并报告失败。
func (l *Listener) persist(ctx context.Context, gen uint64, room int64, write func() (bool, error)) error {
	phase := l.Snapshot().Phase
	if phase == PhaseStorageError {
		phase = PhaseConnecting
	}
	for {
		changed, err := write()
		if err == nil {
			l.publish(gen, phase, room, nil, changed)
			return nil
		}
		l.publish(gen, PhaseStorageError, room, err, false)
		if !sleep(ctx, l.retryDelay) {
			_, finalErr := write()
			return finalErr
		}
	}
}

func (l *Listener) save(ctx context.Context, gen uint64, room int64, write func() (bool, error)) error {
	if len(l.pending) > 0 {
		// 取消导致失败的写入入队后，即使磁盘恢复，
		// 也要在处理已解码帧的剩余部分时保持接收顺序。
		l.pending = append(l.pending, pendingWrite{room: room, write: write})
		return nil
	}
	err := l.persist(ctx, gen, room, write)
	if err != nil {
		l.pending = append(l.pending, pendingWrite{room: room, write: write})
	}
	// 已取消的线格式读取器必须完成已读取帧的解码。
	// 待写入操作保留在内存中，并在下一个会话开始前写完；
	// 最终关闭时的失败由 Close 返回，绝不计作已提交。
	return nil
}

func (l *Listener) run(ctx context.Context, next target, gen uint64) {
	c, err := l.factory(next.proxy, next.account)
	if err != nil {
		l.publish(gen, PhaseError, 0, err, true)
		return
	}
	if client, ok := c.(*bili.Client); ok {
		defer client.HTTP.CloseIdleConnections()
	}
	var room int64
	attempt, failures := 0, 0
	for ctx.Err() == nil {
		l.publish(gen, PhaseConnecting, room, nil, false)
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
			// 每次会话边界都要持久化，包括正常重启。服务器没有
			// 重放游标，因此重连绝不意味着消息记录完整无缺。
			err = l.save(ctx, gen, room, func() (bool, error) { return true, l.history.RecordGap(room, "session_start") })
			if err != nil || ctx.Err() != nil {
				return
			}
			l.publish(gen, PhaseConnecting, room, nil, false)
			started := time.Now()
			authenticated := false
			err = c.ListenDanmaku(ctx, room, attempt, func() {
				authenticated = true
				l.publish(gen, PhaseConnected, room, nil, false)
			}, func(raw json.RawMessage) error {
				return l.save(ctx, gen, room, func() (bool, error) { return l.history.Append(room, raw) })
			})
			// 不要仅因认证成功就重置退避：否则反复断开的连接
			// 会导致重连风暴。
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
		l.publish(gen, PhaseReconnecting, room, err, true)
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
