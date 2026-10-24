//go:build qt

package main

import (
	"flag"

	"arcana-world/internal/overlay"
	"arcana-world/internal/overlay/qt"
)

var runBackend = qt.Run
var listBackendDisplays = qt.ListDisplays

const backendID = overlay.BackendQt

const backendHelp = "Elegant overlay: macOS, Windows and Linux. Wayland requires layer-shell; GNOME/Mutter is unsupported."

func registerBackendFlags(flags *flag.FlagSet, cfg *overlay.Config) {
	q := &cfg.Qt
	flags.StringVar(&q.Alignment, "align", q.Alignment, "Elegant overlay text alignment: left, center, right")
	flags.Float64Var(&q.HistoryScale, "history-scale", q.HistoryScale, "Elegant overlay history font scale (0.5..1.5)")
	flags.Float64Var(&q.LatestScale, "latest-scale", q.LatestScale, "Elegant overlay latest message font scale (0.5..2)")
	flags.Float64Var(&q.LineSpacing, "line-spacing", q.LineSpacing, "Elegant overlay line spacing in logical points (0..32)")
	flags.Float64Var(&q.MessageSpacing, "message-spacing", q.MessageSpacing, "Elegant overlay message spacing in logical points (0..64)")
	flags.Float64Var(&q.HistoryAlpha, "history-alpha", q.HistoryAlpha, "Elegant overlay history opacity multiplier (0..1)")
	flags.IntVar(&q.MaxMessages, "max-messages", q.MaxMessages, "Elegant overlay visible message limit (1..6)")
	flags.IntVar(&q.MaxLines, "max-lines", q.MaxLines, "Elegant overlay wrapped lines per message (1..12); overflow ends with an ellipsis; only whole messages fit")
	flags.BoolVar(&q.Gradient, "gradient", q.Gradient, "Elegant overlay latest message accent gradient")
	flags.StringVar(&q.AccentStart, "accent-start", q.AccentStart, "Elegant overlay gradient start color (#RRGGBB)")
	flags.StringVar(&q.AccentEnd, "accent-end", q.AccentEnd, "Elegant overlay gradient end color (#RRGGBB)")
	flags.Float64Var(&q.CornerRadius, "radius", q.CornerRadius, "Elegant overlay panel radius in logical points (0..64)")
	flags.Float64Var(&q.BorderAlpha, "border-alpha", q.BorderAlpha, "Elegant overlay border opacity (0..1)")
	flags.Float64Var(&q.ShadowStrength, "shadow", q.ShadowStrength, "Elegant overlay shadow strength (0..1)")
	flags.BoolVar(&q.AnimationEnabled, "animation", q.AnimationEnabled, "Elegant overlay message animation")
	flags.IntVar(&q.AnimationDurationMS, "animation-ms", q.AnimationDurationMS, "Elegant overlay animation duration in milliseconds (0..2000); 0 disables animation")
	flags.Float64Var(&q.AnimationDistance, "animation-distance", q.AnimationDistance, "Elegant overlay animation distance in logical points (0..128); 0 fades only")
	flags.BoolVar(&q.IdleHideEnabled, "idle-hide", q.IdleHideEnabled, "Elegant overlay fades out when the text stays unchanged and fades back in on new text")
	flags.IntVar(&q.IdleHideSeconds, "idle-seconds", q.IdleHideSeconds, "Elegant overlay seconds without new text before fading out (1..600)")
}

func configureBackend(reducedMotion, unlocked bool) error {
	qt.Configure(reducedMotion, unlocked)
	return nil
}
