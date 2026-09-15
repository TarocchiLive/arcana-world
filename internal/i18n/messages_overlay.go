package i18n

// 浮层是可选展示端，启动或运行失败不阻断 TUI。
const (
	CLIOverlayUsage            Key = "overlay.cli.enable"
	CLIOverlayExecutableUsage  Key = "overlay.cli.executable"
	TUILogOverlayStarted       Key = "overlay.log.started"
	TUILogOverlayStartupFailed Key = "overlay.log.startup_failed"
	TUILogOverlayStopped       Key = "overlay.log.stopped"
	TUILogOverlayUpdateFailed  Key = "overlay.log.update_failed"
)
