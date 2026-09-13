// SPDX-License-Identifier: GPL-3.0-only
package coverimage

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareCenteredCropAndResize(t *testing.T) {
	for _, size := range []image.Point{{160, 160}, {320, 90}, {32, 18}, {3, 7}} {
		t.Run(size.String(), func(t *testing.T) {
			source := image.NewNRGBA(image.Rect(0, 0, size.X, size.Y))
			for y := range size.Y {
				for x := range size.X {
					source.SetNRGBA(x, y, color.NRGBA{R: uint8(x * 255 / (size.X - 1)), G: uint8(y * 255 / (size.Y - 1)), A: 255})
				}
			}
			var raw bytes.Buffer
			if err := png.Encode(&raw, source); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "source.png")
			if err := os.WriteFile(path, raw.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			p, err := Prepare(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := png.Decode(bytes.NewReader(p.PNG))
			if err != nil {
				t.Fatal(err)
			}
			if decoded.Bounds() != image.Rect(0, 0, 704, 396) {
				t.Fatalf("upload dimensions: %v", decoded.Bounds())
			}
			if p.SourceWidth != size.X || p.SourceHeight != size.Y || p.Cropped != (size.X*9 != size.Y*16) {
				t.Fatalf("incorrect crop guidance: %+v", size)
			}
			for _, point := range []image.Point{{0, 0}, {352, 198}, {703, 395}} {
				if color.NRGBAModel.Convert(decoded.At(point.X, point.Y)) != color.NRGBAModel.Convert(p.Image.At(point.X, point.Y)) {
					t.Fatal("preview differs from uploaded pixels")
				}
			}
			center := color.NRGBAModel.Convert(decoded.At(352, 198)).(color.NRGBA)
			if center.R < 120 || center.R > 136 || center.G < 120 || center.G > 136 {
				t.Fatalf("crop not centered: %v", center)
			}
			if size.X == 160 {
				top := color.NRGBAModel.Convert(decoded.At(352, 0)).(color.NRGBA)
				if top.G < 50 || top.G > 62 {
					t.Fatalf("square image not center-cropped: %v", top)
				}
			}
			unchanged, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(unchanged, raw.Bytes()) {
				t.Fatal("original image was changed")
			}
		})
	}
}

func TestDecodeLimits(t *testing.T) {
	if _, err := Decode(make([]byte, MaxFileSize+1)); err == nil {
		t.Fatal("accepted oversized input")
	}
	var raw bytes.Buffer
	// A highly compressible image crosses the pixel bound without a large file.
	if err := png.Encode(&raw, image.NewGray(image.Rect(0, 0, 5001, 5000))); err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(raw.Bytes()); err == nil {
		t.Fatal("accepted more than 25 million pixels")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Prepare(ctx, "unused"); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}
