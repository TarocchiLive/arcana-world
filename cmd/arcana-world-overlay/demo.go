package main

import (
	"context"
	"strings"
	"time"

	"arcana-world/internal/overlay"
)

// 两种后端共用同一组示例，便于比较文字排版和动态更新。
var demoLines = [...]string{
	"小林：晚上好，今天的界面好漂亮",
	"月见：这层半透明面板很舒服",
	"星河：中文和 English 都能清晰显示",
	"Mika：新消息的入场动画好丝滑",
	"南风：支持一下，送出一颗小星星",
	"Ling：这就是 Arcana World 的新弹幕浮层",
}

func streamDemo(ctx context.Context, cfg overlay.Config, updates chan<- overlay.Config) {
	defer close(updates)
	ticker := time.NewTicker(1800 * time.Millisecond)
	defer ticker.Stop()
	// 保留最近六条，不插入空白帧；最后内容一直显示到下一次更新。
	lines := make([]string, 1, len(demoLines))
	lines[0] = demoLines[0]
	index := 1
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if len(lines) == cap(lines) {
				copy(lines, lines[1:])
				lines = lines[:len(lines)-1]
			}
			lines = append(lines, demoLines[index])
			index = (index + 1) % len(demoLines)
			next := cfg
			next.Text = strings.Join(lines, "\n")
			next.TextRoles = strings.Repeat("n", len(lines))
			select {
			case updates <- next:
			case <-ctx.Done():
				return
			}
		}
	}
}
