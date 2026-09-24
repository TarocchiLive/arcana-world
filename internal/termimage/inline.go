package termimage

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"strings"

	"golang.org/x/image/draw"
)

// Thumbnail 保存中心裁剪后的方形像素和 PNG；加载时编码，不在绘制时重复编码。
type Thumbnail struct {
	pixels *image.NRGBA
	png    []byte
}

// NewThumbnail 保留最多 256×256 像素，小图不放大。
func NewThumbnail(img image.Image) (*Thumbnail, error) {
	if img == nil {
		return nil, errors.New("thumbnail: nil image")
	}
	bounds := img.Bounds()
	side := min(bounds.Dx(), bounds.Dy())
	if side <= 0 {
		return nil, errors.New("thumbnail: empty image")
	}
	left := bounds.Min.X + (bounds.Dx()-side)/2
	top := bounds.Min.Y + (bounds.Dy()-side)/2
	crop := image.Rect(left, top, left+side, top+side)
	size := min(side, 256)
	pixels := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.CatmullRom.Scale(pixels, pixels.Bounds(), img, crop, draw.Src, nil)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, pixels); err != nil {
		return nil, err
	}
	return &Thumbnail{pixels: pixels, png: encoded.Bytes()}, nil
}

// Fallback 用上下半像素填满预留区域；调用方仅在原生图形不可用时使用。
// 每行重置颜色且末尾不换行，便于嵌入布局；调用方可按尺寸缓存结果。
func (t *Thumbnail) Fallback(cols, rows int) string {
	if t == nil || t.pixels == nil || cols <= 0 || rows <= 0 {
		return ""
	}
	// 先以面积对应的滤波缩放采样，避免直接取稀疏像素丢失头像细节。
	pixels := image.NewNRGBA(image.Rect(0, 0, cols, 2*rows))
	draw.CatmullRom.Scale(pixels, pixels.Bounds(), t.pixels, t.pixels.Bounds(), draw.Src, nil)
	var out strings.Builder
	for y := range rows {
		if y > 0 {
			out.WriteByte('\n')
		}
		for x := range cols {
			r1, g1, b1, _ := pixels.At(x, 2*y).RGBA()
			r2, g2, b2, _ := pixels.At(x, 2*y+1).RGBA()
			fmt.Fprintf(&out, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", r1>>8, g1>>8, b1>>8, r2>>8, g2>>8, b2>>8)
		}
		out.WriteString("\x1b[0m")
	}
	return out.String()
}

// Upload 复用分块传输；q=0 要求调用方等待该 id 的成功 ACK 后再放置。
func (t *Thumbnail) Upload(id uint32) string {
	if t == nil || len(t.png) == 0 {
		return ""
	}
	var out bytes.Buffer
	_ = transmit(&out, t.png, id) // bytes.Buffer 写入不会失败。
	return out.String()
}

// GraphicsSupported 仅做环境预筛选，最终支持情况必须由 Probe 的 ACK 确认。
func GraphicsSupported() bool {
	return supportsGraphics(os.Getenv)
}

// Probe 查询直接传输能力；查询不创建持久图像，响应由终端事件循环接收。
func Probe(id uint32) string {
	return fmt.Sprintf("\x1b_Ga=q,t=d,f=24,s=1,v=1,i=%d,q=0;AAAA\x1b\\", id)
}

// Place 在零基屏幕坐标放置方形图片，保存并恢复光标且禁止移动光标。
// 先移除旧放置但保留图像数据；调用方用 placementID 区分过期 ACK。
// 只限制行数以保持像素宽高比；q=0 的放置 ACK 也必须由调用方处理。
func Place(id, placementID uint32, x, y, rows int) string {
	if x < 0 || y < 0 || rows <= 0 {
		return ""
	}
	return fmt.Sprintf("\x1b_Ga=d,d=i,i=%d,q=2;\x1b\\\x1b7\x1b[%d;%dH\x1b_Ga=p,i=%d,p=%d,r=%d,C=1,q=0;\x1b\\\x1b8", id, y+1, x+1, id, placementID, rows)
}

// Delete 只删除该 id 的所有放置和图像数据，不干扰其他图片。
func Delete(id uint32) string {
	var out bytes.Buffer
	_ = deleteImage(&out, id)
	return out.String()
}

// CellGeometry 根据单元格像素尺寸预留方形所需列数；未知尺寸按宽高比 0.5。
func CellGeometry(rows, cellWidth, cellHeight int) int {
	if rows <= 0 {
		return 0
	}
	aspect := defaultCellAspect
	if cellWidth > 0 && cellHeight > 0 {
		aspect = float64(cellWidth) / float64(cellHeight)
	}
	return max(1, int(math.Ceil(float64(rows)/aspect)))
}
