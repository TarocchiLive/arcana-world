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
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	bounds := source.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	output := image.NewNRGBA(image.Rect(0, 0, Width, Height))
	// 统一的仿射缩放即使面对极小或奇数尺寸源图，也能精确保留宽高比，
	// 不会出现先将裁剪矩形取整再缩放所造成的比例偏差。
	scale := math.Max(float64(Width)/float64(w), float64(Height)/float64(h))
	transform := f64.Aff3{scale, 0, (Width-scale*float64(w))/2 - scale*float64(bounds.Min.X), 0, scale, (Height-scale*float64(h))/2 - scale*float64(bounds.Min.Y)}
	draw.CatmullRom.Transform(output, transform, source, bounds, draw.Src, nil)
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, output); err != nil {
		return nil, errors.New(i18n.T(i18n.CoverImagePngEncodeFailed))
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return &Prepared{PNG: encoded.Bytes(), Image: output, SourceWidth: w, SourceHeight: h, Cropped: int64(w)*9 != int64(h)*16}, nil
}
