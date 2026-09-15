package overlay

import (
	"strings"
	"testing"
)

func TestRepeatedSnapshotDoesNotAllocate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Text = strings.Repeat("x", MaxTextBytes)
	// 单独测量发布热路径，避免子进程 I/O 的异步分配影响结果。
	manager := &Manager{cfg: cfg, dirty: make(chan struct{}, 1)}
	allocations := testing.AllocsPerRun(100, func() {
		if err := manager.SetConfig(manager.Snapshot()); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("unchanged snapshot allocated %.0f objects per submission", allocations)
	}
}
