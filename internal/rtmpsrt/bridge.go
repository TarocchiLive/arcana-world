package rtmpsrt

import (
	"context"
	"errors"
	"io"
	"net"
	"sync"

	"arcana-world/internal/journal"
)

var logger = journal.Logger("rtmpsrt")

// Available 只在用户选择 RTMPSRT 时加载可选原生库。
func Available() error { _, err := loadSRT(); return err }

// Bridge 的入口仅绑定 IPv4 回环地址，不对局域网开放发布凭据。
type Bridge struct {
	listener      net.Listener
	target        Target
	api           *nativeAPI
	ctx           context.Context
	cancel        context.CancelFunc
	mu            sync.Mutex
	closed        bool
	active        map[io.Closer]struct{}
	done          chan struct{}
	onError       func(error)
	publishCancel context.CancelFunc
}

func New(target Target, onError func(error)) (*Bridge, error) {
	api, err := loadSRT()
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		logger.Error("bridge listener failed", "stage", "listen", "err", err)
		return nil, errors.New("RTMPSRT local listener failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	b := &Bridge{listener: listener, target: target, api: api, ctx: ctx, cancel: cancel, active: make(map[io.Closer]struct{}), done: make(chan struct{}), onError: onError}
	go b.run()
	return b, nil
}
func (b *Bridge) Server() string        { return "rtmp://" + b.listener.Addr().String() + "/" + b.target.App }
func (b *Bridge) Key() string           { return b.target.Playpath }
func (b *Bridge) Done() <-chan struct{} { return b.done }
func (b *Bridge) track(c io.Closer) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		c.Close()
		return false
	}
	b.active[c] = struct{}{}
	return true
}
func (b *Bridge) untrack(c io.Closer) { c.Close(); b.mu.Lock(); delete(b.active, c); b.mu.Unlock() }

// Stop 只回收当前发布会话，保留 OBS 已配置的本地入口供手动重新开播。
func (b *Bridge) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.publishCancel != nil {
		b.publishCancel()
	}
	for c := range b.active {
		c.Close()
	}
}

// Close 不等待回调，允许 OBS 在自己的状态锁内关闭；Done 可等待全部工作协程退出。
func (b *Bridge) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	b.cancel()
	b.listener.Close()
	for c := range b.active {
		c.Close()
	}
	return nil
}
func (b *Bridge) run() {
	defer close(b.done)
	for {
		local, err := b.listener.Accept()
		if err != nil {
			if b.ctx.Err() == nil {
				logger.Error("bridge accept failed", "stage", "accept", "err", err)
				if b.onError != nil {
					b.onError(err)
				}
			}
			return
		}
		ctx, cancel := context.WithCancel(b.ctx)
		b.mu.Lock()
		if b.closed {
			b.mu.Unlock()
			cancel()
			local.Close()
			return
		}
		b.publishCancel = cancel
		b.active[local] = struct{}{}
		b.mu.Unlock()
		err = b.serve(ctx, local)
		failed := err != nil && ctx.Err() == nil
		cancel()
		b.untrack(local)
		b.mu.Lock()
		b.publishCancel = nil
		b.mu.Unlock()
		if failed && b.ctx.Err() == nil && b.onError != nil {
			b.onError(err)
		}
	}
}
func (b *Bridge) serve(ctx context.Context, local net.Conn) error {
	remote, err := dialSRT(ctx, b.api, b.target)
	if err != nil {
		if ctx.Err() == nil {
			logger.Error("bridge connection failed", "stage", "srt_dial", "host", b.target.Host, "err", err)
		}
		return err
	}
	stop := context.AfterFunc(ctx, func() { local.Close(); remote.Close() })
	defer stop()
	if !b.track(remote) {
		return b.ctx.Err()
	}
	defer b.untrack(remote)
	mapping := &ackMap{}
	source := &countedReader{Reader: local}
	destination := &mappedWriter{Writer: remote, source: source, mapping: mapping}
	reverse := make(chan error, 1)
	go func() { err := relayReplies(local, remote, mapping); local.Close(); remote.Close(); reverse <- err }()
	err = relayRTMP(destination, source, b.target)
	local.Close()
	remote.Close()
	reverseErr := <-reverse
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		err = reverseErr
	}
	if err != nil && ctx.Err() == nil {
		logger.Error("bridge relay failed", "stage", "relay", "host", b.target.Host, "err", err, "reply_err", reverseErr)
	}
	return err
}
