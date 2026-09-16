package overlay

import (
	"errors"
	"math"
	"strings"
	"unicode/utf8"
)

// MaxTextBytes 限制单个状态的文本大小；协议另有包含 JSON 转义开销的帧上限。
const MaxTextBytes = 1024 * 1024

// Anchor 表示显示器完整边界上的九宫格锚点，不受工作区或前台窗口影响。
type Anchor string

const (
	TopLeft     Anchor = "top-left"
	Top         Anchor = "top"
	TopRight    Anchor = "top-right"
	Left        Anchor = "left"
	Center      Anchor = "center"
	Right       Anchor = "right"
	BottomLeft  Anchor = "bottom-left"
	Bottom      Anchor = "bottom"
	BottomRight Anchor = "bottom-right"
)

// Index 按从左到右、从上到下的顺序返回原生桥接使用的编号；无效值返回 -1。
func (a Anchor) Index() int {
	switch a {
	case TopLeft:
		return 0
	case Top:
		return 1
	case TopRight:
		return 2
	case Left:
		return 3
	case Center:
		return 4
	case Right:
		return 5
	case BottomLeft:
		return 6
	case Bottom:
		return 7
	case BottomRight:
		return 8
	default:
		return -1
	}
}

// Position 的偏移单位为逻辑点。靠左/上时正值向右/下，靠右/下时正值向内；
// 居中轴的正值向右/下。允许负偏移，但最终窗口始终限制在显示器内。
type Position struct {
	Anchor Anchor  `json:"anchor"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
}

// Insets 是以逻辑点为单位的四边内边距；超过窗口尺寸时内容区域为空。
type Insets struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

// Font 的空 Family 使用平台默认字体；字形回退由系统字体配置决定。
// Weight 使用 100..900 的常见字重标度，Size 使用逻辑点。
type Font struct {
	Family string  `json:"family"`
	Size   float64 `json:"size"`
	Weight int     `json:"weight"`
	Italic bool    `json:"italic"`
}

// Config 是可比较的完整展示快照，不含共享可变内存。
// 默认显示器按初始选择保留，不跟随前台应用。显示器断开时可临时回退，重连后恢复。
type Config struct {
	Text            string   `json:"text"`
	Position        Position `json:"position"`
	Width           float64  `json:"width"`
	Height          float64  `json:"height"`
	Padding         Insets   `json:"padding"`
	Font            Font     `json:"font"`
	TextAlpha       float64  `json:"text_alpha"`
	BackgroundAlpha float64  `json:"background_alpha"`
	// DisplayID 是 macOS 原生 ID 或 Windows 从 1 开始的枚举序号；Wayland 使用 Output。
	DisplayID uint32 `json:"display_id"`
	// Output 是 Wayland 输出名称或 Windows 设备名称；不能与 DisplayID 同时指定。
	Output string `json:"output"`
}

func DefaultConfig() Config {
	return Config{Text: "Arcana World", Position: Position{Anchor: Left, X: 0, Y: 0}, Width: 420, Height: 180, Padding: Insets{Top: 12, Right: 16, Bottom: 12, Left: 16}, Font: Font{Size: 16, Weight: 500}, TextAlpha: .9, BackgroundAlpha: .3}
}

// Normalize 在进入协议或原生代码前校验所有边界，并统一修复用户可见文本。
// 位置和尺寸在解析显示器后还会被限制；NaN/Inf 不得进入原生数值转换。
func (cfg Config) Normalize() (Config, error) {
	for _, value := range [...]float64{cfg.Position.X, cfg.Position.Y, cfg.Width, cfg.Height, cfg.Padding.Top, cfg.Padding.Right, cfg.Padding.Bottom, cfg.Padding.Left, cfg.Font.Size, cfg.TextAlpha, cfg.BackgroundAlpha} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return Config{}, errors.New("overlay: geometry and opacity must be finite")
		}
	}
	if cfg.Position.Anchor.Index() < 0 {
		return Config{}, errors.New("overlay: invalid anchor")
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Font.Size <= 0 || cfg.Font.Size > 32768 {
		return Config{}, errors.New("overlay: require positive dimensions and font size in (0, 32768]")
	}
	if cfg.Padding.Top < 0 || cfg.Padding.Right < 0 || cfg.Padding.Bottom < 0 || cfg.Padding.Left < 0 {
		return Config{}, errors.New("overlay: padding must be nonnegative")
	}
	if cfg.Font.Weight < 100 || cfg.Font.Weight > 900 {
		return Config{}, errors.New("overlay: font weight must be in 100..900")
	}
	if cfg.TextAlpha < 0 || cfg.TextAlpha > 1 || cfg.BackgroundAlpha < 0 || cfg.BackgroundAlpha > 1 {
		return Config{}, errors.New("overlay: opacity must be in 0..1")
	}
	if cfg.DisplayID != 0 && cfg.Output != "" {
		return Config{}, errors.New("overlay: use either display ID or output name, not both")
	}
	for _, name := range [...]string{cfg.Output, cfg.Font.Family} {
		if len(name) > 256 || strings.ContainsRune(name, '\x00') || !utf8.ValidString(name) {
			return Config{}, errors.New("overlay: output and font names must be valid UTF-8 without NUL, at most 256 bytes")
		}
	}
	// 先拒绝超限输入，避免清理过程中复制无界字符串；修复后的字节数也要检查。
	if len(cfg.Text) > MaxTextBytes {
		return Config{}, errors.New("overlay: text exceeds the 1 MiB rendering limit")
	}
	cfg.Text = strings.ToValidUTF8(strings.ReplaceAll(cfg.Text, "\x00", ""), "�")
	if len(cfg.Text) > MaxTextBytes {
		return Config{}, errors.New("overlay: repaired text exceeds the 1 MiB rendering limit")
	}
	return cfg, nil
}

// Rect 使用从左上角向右、向下递增的逻辑坐标。
type Rect struct{ X, Y, Width, Height float64 }

// Frame 将有效配置解析到完整显示器边界，先限制浮点值，再交由后端转换为像素。
func (cfg Config) Frame(bounds Rect) Rect {
	width, height := math.Min(cfg.Width, bounds.Width), math.Min(cfg.Height, bounds.Height)
	index := cfg.Position.Anchor.Index()
	return Rect{X: bounds.X + anchorOffset(index%3, cfg.Position.X, bounds.Width-width), Y: bounds.Y + anchorOffset(index/3, cfg.Position.Y, bounds.Height-height), Width: width, Height: height}
}
func anchorOffset(axis int, offset, available float64) float64 {
	switch axis {
	case 1:
		offset = available/2 + offset
	case 2:
		offset = available - offset
	}
	return math.Max(0, math.Min(offset, available))
}

// ContentRect 对超大内边距做饱和处理，不产生负尺寸或整型溢出。
func (cfg Config) ContentRect(width, height float64) Rect {
	x, y := math.Min(cfg.Padding.Left, width), math.Min(cfg.Padding.Top, height)
	return Rect{X: x, Y: y, Width: math.Max(0, width-x-math.Min(cfg.Padding.Right, width)), Height: math.Max(0, height-y-math.Min(cfg.Padding.Bottom, height))}
}
