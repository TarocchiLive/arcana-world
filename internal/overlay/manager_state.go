package overlay

import "errors"

// setLocked 只在持锁时提交值快照。不要改回 func(*Config) 回调：
// 将局部快照地址传给任意函数会使每次提交逃逸到堆，连重复状态也会分配。
func (m *Manager) setLocked(cfg Config) error {
	if m.closed {
		return errors.New("overlay: manager is closed")
	}
	if cfg == m.cfg {
		return nil
	}
	normalized, err := cfg.Normalize()
	if err != nil {
		return err
	}
	if normalized == m.cfg {
		return nil
	}
	m.cfg = normalized
	select {
	case m.dirty <- struct{}{}:
	default:
	}
	return nil
}

func (m *Manager) SetConfig(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.setLocked(cfg)
}
func (m *Manager) Snapshot() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}
func (m *Manager) SetText(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg
	cfg.Text = text
	return m.setLocked(cfg)
}
func (m *Manager) SetPosition(position Position) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg
	cfg.Position = position
	return m.setLocked(cfg)
}
func (m *Manager) SetSize(width, height float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg
	cfg.Width, cfg.Height = width, height
	return m.setLocked(cfg)
}
func (m *Manager) SetPadding(padding Insets) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg
	cfg.Padding = padding
	return m.setLocked(cfg)
}
func (m *Manager) SetFont(font Font) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg
	cfg.Font = font
	return m.setLocked(cfg)
}
func (m *Manager) SetOpacity(text, background float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg
	cfg.TextAlpha, cfg.BackgroundAlpha = text, background
	return m.setLocked(cfg)
}
func (m *Manager) SetDisplay(id uint32, output string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cfg := m.cfg
	cfg.DisplayID, cfg.Output = id, output
	return m.setLocked(cfg)
}

// Place 在显示器完整边界上定位；正偏移的方向由 Position 的锚点语义决定。
func (m *Manager) Place(anchor Anchor, x, y float64) error {
	return m.SetPosition(Position{Anchor: anchor, X: x, Y: y})
}
