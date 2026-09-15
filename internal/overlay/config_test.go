package overlay

import (
	"math"
	"strings"
	"testing"
)

func TestFrameAnchorsRespectFullDisplayBounds(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Width, cfg.Height = 200, 100
	bounds := Rect{X: -800, Y: 50, Width: 1000, Height: 600}
	for _, test := range []struct {
		anchor Anchor
		x, y   float64
	}{
		{TopLeft, -790, 70}, {Top, -390, 70}, {TopRight, -10, 70},
		{Left, -790, 320}, {Center, -390, 320}, {Right, -10, 320},
		{BottomLeft, -790, 530}, {Bottom, -390, 530}, {BottomRight, -10, 530},
	} {
		t.Run(string(test.anchor), func(t *testing.T) {
			cfg.Position = Position{Anchor: test.anchor, X: 10, Y: 20}
			want := Rect{X: test.x, Y: test.y, Width: 200, Height: 100}
			if got := cfg.Frame(bounds); got != want {
				t.Fatalf("frame=%+v, want %+v", got, want)
			}
		})
	}
	cfg.Width, cfg.Height = 1e30, 1e30
	cfg.Position = Position{Anchor: BottomRight, X: -1e30, Y: 1e30}
	if got := cfg.Frame(bounds); got != bounds {
		t.Fatalf("oversized frame escaped bounds: %+v", got)
	}
}

func TestContentRectSaturatesPadding(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Padding = Insets{Top: 5, Right: 20, Bottom: 10, Left: 15}
	if got := cfg.ContentRect(100, 80); got != (Rect{X: 15, Y: 5, Width: 65, Height: 65}) {
		t.Fatalf("asymmetric padding: %+v", got)
	}
	cfg.Padding = Insets{Top: 1e30, Right: 1e30, Bottom: 1e30, Left: 1e30}
	if got := cfg.ContentRect(1, 1); got != (Rect{X: 1, Y: 1}) {
		t.Fatalf("excessive padding: %+v", got)
	}
}

func TestNormalizeRejectsUnsafeNativeInputs(t *testing.T) {
	for _, change := range []func(*Config){
		func(c *Config) { c.Position.X = math.Inf(1) },
		func(c *Config) { c.Padding.Top = math.NaN() },
		func(c *Config) { c.Font.Weight = 901 },
		func(c *Config) { c.Font.Family = "font\x00hidden" },
		func(c *Config) { c.Output = "\xff" },
		func(c *Config) { c.DisplayID = 1; c.Output = "Virtual-1" },
		func(c *Config) { c.Position.Anchor = "unknown" },
		func(c *Config) { c.Text = strings.Repeat("\xffa", MaxTextBytes/4+1) },
	} {
		cfg := DefaultConfig()
		change(&cfg)
		if _, err := cfg.Normalize(); err == nil {
			t.Fatal("accepted unsafe configuration")
		}
	}
	cfg := DefaultConfig()
	cfg.Text = "中文\x00\xff"
	normalized, err := cfg.Normalize()
	if err != nil || normalized.Text != "中文�" {
		t.Fatalf("normalization=%q, %v", normalized.Text, err)
	}
}
