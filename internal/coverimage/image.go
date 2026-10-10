// SPDX-License-Identifier: GPL-3.0-only
package coverimage

import (
	"arcana-world/internal/i18n"

	"bytes"
	"context"
	"errors"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"math"
	"os"

	"golang.org/x/image/draw"
	"golang.org/x/image/math/f64"
)

const (
	Width       = 704
	Height      = 396
	MaxFileSize = 20 << 20
	maxPixels   = 25_000_000
)

type Prepared struct {
	PNG                       []byte
	Image                     image.Image
	SourceWidth, SourceHeight int
	Cropped                   bool
	source                    image.Image
	zoom                      int
	centerX, centerY          float64
}

// Decode 在分配像素内存前限制压缩数据大小和解码后的像素总数。
// GIF 解码有意只使用首帧。
func Decode(raw []byte) (image.Image, error) {
	if len(raw) == 0 || len(raw) > MaxFileSize {
		return nil, errors.New(i18n.T(i18n.CoverImageFileSizeInvalid))
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "png" && format != "jpeg" && format != "gif") || config.Width <= 0 || config.Height <= 0 || int64(config.Width) > maxPixels/int64(config.Height) {
		return nil, errors.New(i18n.T(i18n.CoverImageImageFormatInvalid))
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New(i18n.T(i18n.CoverImageDecodeFailed))
	}
	return img, nil
}

func Prepare(ctx context.Context, path string) (*Prepared, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 先调用 Stat，避免传入 FIFO 路径时阻塞；打开后再次检查文件描述符，
	// 确保不会读取被替换的文件或非普通文件。
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxFileSize {
		return nil, errors.New(i18n.T(i18n.CoverImageRegularFileRequired))
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New(i18n.T(i18n.CoverImageFileOpenFailed))
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) || opened.Size() > MaxFileSize {
		return nil, errors.New(i18n.T(i18n.CoverImageFileChanged))
	}
	raw, err := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
	if err != nil {
		return nil, errors.New(i18n.T(i18n.CoverImageFileReadFailed))
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	source, err := Decode(raw)
	if err != nil {
		return nil, err
	}
	prepared := &Prepared{source: source, SourceWidth: source.Bounds().Dx(), SourceHeight: source.Bounds().Dy()}
	return prepared.Reframe(ctx, 1, 0.5, 0.5)
}

// Framing 返回相对自动适配的倍率，以及取景中心在原图中的比例坐标。
func (p *Prepared) Framing() (zoom int, centerX, centerY float64) {
	return p.zoom, p.centerX, p.centerY
}

// Reframe 从保留的原图重新取景，输出固定为 704×396；旧结果和原图均不修改。
// 中心坐标自动限制在原图内，缩小后也不会留下空白边缘。
func (p *Prepared) Reframe(ctx context.Context, zoom int, centerX, centerY float64) (*Prepared, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if p == nil || p.source == nil || zoom < 1 || zoom > 8 ||
		math.IsNaN(centerX) || math.IsNaN(centerY) || math.IsInf(centerX, 0) || math.IsInf(centerY, 0) {
		return nil, errors.New(i18n.T(i18n.CoverImageFramingInvalid))
	}
	bounds := p.source.Bounds()
	w, h := float64(bounds.Dx()), float64(bounds.Dy())
	scale := math.Max(float64(Width)/w, float64(Height)/h) * float64(zoom)
	halfWidth, halfHeight := float64(Width)/scale/w/2, float64(Height)/scale/h/2
	centerX = max(halfWidth, min(1-halfWidth, centerX))
	centerY = max(halfHeight, min(1-halfHeight, centerY))
	if zoom == p.zoom && centerX == p.centerX && centerY == p.centerY {
		return p, nil
	}
	output := image.NewNRGBA(image.Rect(0, 0, Width, Height))
	// 使用浮点取景中心，避免奇数尺寸或极小源图因裁剪取整而改变比例。
	transform := f64.Aff3{scale, 0, float64(Width)/2 - scale*(float64(bounds.Min.X)+centerX*w), 0, scale, float64(Height)/2 - scale*(float64(bounds.Min.Y)+centerY*h)}
	draw.CatmullRom.Transform(output, transform, p.source, bounds, draw.Src, nil)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, output); err != nil {
		return nil, errors.New(i18n.T(i18n.CoverImagePngEncodeFailed))
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	next := *p
	next.PNG, next.Image = encoded.Bytes(), output
	next.zoom, next.centerX, next.centerY = zoom, centerX, centerY
	next.Cropped = zoom > 1 || int64(p.SourceWidth)*9 != int64(p.SourceHeight)*16
	return &next, nil
}
