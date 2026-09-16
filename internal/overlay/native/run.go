// Package native 在浮层子进程的主 OS 线程上运行平台图形循环。
// 核心程序只依赖父包 overlay，避免将 cgo 和图形库引入 TUI。
package native

import (
	"arcana-world/internal/overlay"
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var started atomic.Bool

// Run 每进程仅调用一次，且调用者必须锁定主 OS 线程。
// updates 是完整状态快照；关闭通道保留最后状态，上下文取消则退出。
// ready 在初始原生窗口/表面及首帧提交成功后通知，回调不得阻塞 UI 线程。
func Run(ctx context.Context, cfg overlay.Config, updates <-chan overlay.Config, ready func()) error {
	cfg, err := cfg.Normalize()
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if !started.CompareAndSwap(false, true) {
		return errors.New("overlay: native host may run only once per process")
	}
	var once sync.Once
	notify := func() {
		if ready != nil {
			once.Do(ready)
		}
	}
	return runPlatform(ctx, cfg, updates, notify)
}
