// SPDX-License-Identifier: GPL-3.0-only
package coverimage

import (
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

// Decode bounds compressed size and decoded pixel count before allocating pixels.
// GIF decoding deliberately uses only its first frame.
func Decode(raw []byte) (image.Image, error) {
	if len(raw) == 0 || len(raw) > MaxFileSize {
		return nil, errors.New("封面文件必须为 1 字节至 20 MiB")
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || (format != "png" && format != "jpeg" && format != "gif") || config.Width <= 0 || config.Height <= 0 || int64(config.Width) > maxPixels/int64(config.Height) {
		return nil, errors.New("封面格式无效或超过 2500 万像素（支持 PNG、JPEG、GIF）")
	}
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, errors.New("封面图片解码失败")
	}
	return img, nil
}

func Prepare(ctx context.Context, path string) (*Prepared, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Stat first avoids blocking when the supplied path is a FIFO; the opened
	// descriptor is checked again so a replaced/nonregular file is not read.
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > MaxFileSize {
		return nil, errors.New("封面必须为不超过 20 MiB 的普通图片文件")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.New("无法打开封面文件")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) || opened.Size() > MaxFileSize {
		return nil, errors.New("封面文件已改变或不是有效普通文件")
	}
	raw, err := io.ReadAll(io.LimitReader(file, MaxFileSize+1))
	if err != nil {
		return nil, errors.New("读取封面失败")
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
	// A uniform affine scale retains the exact aspect ratio even for tiny or
	// odd-sized sources, unlike rounding an integer crop rectangle then scaling.
	scale := math.Max(float64(Width)/float64(w), float64(Height)/float64(h))
	transform := f64.Aff3{scale, 0, (Width-scale*float64(w))/2 - scale*float64(bounds.Min.X), 0, scale, (Height-scale*float64(h))/2 - scale*float64(bounds.Min.Y)}
	draw.CatmullRom.Transform(output, transform, source, bounds, draw.Src, nil)
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	var encoded bytes.Buffer
	if err = png.Encode(&encoded, output); err != nil {
		return nil, errors.New("封面 PNG 编码失败")
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return &Prepared{PNG: encoded.Bytes(), Image: output, SourceWidth: w, SourceHeight: h, Cropped: int64(w)*9 != int64(h)*16}, nil
}
