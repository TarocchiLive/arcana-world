package termimage

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func TestThumbnailCentersSquareAtNonzeroOrigin(t *testing.T) {
	// 两侧红色不能进入中心蓝色方形，非零原点用于捕获坐标偏移。
	img := image.NewNRGBA(image.Rect(10, 20, 18, 24))
	for y := 20; y < 24; y++ {
		for x := 10; x < 18; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x >= 12 && x < 16 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	thumbnail, err := NewThumbnail(img)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(thumbnail.png))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 4 || decoded.Bounds().Dy() != 4 {
		t.Fatalf("not a preserved square: %v", decoded.Bounds())
	}
	for y := range 4 {
		for x := range 4 {
			r, g, b, a := decoded.At(x, y).RGBA()
			if r != 0 || g != 0 || b != 65535 || a != 65535 {
				t.Fatalf("crop included outer pixels at %d,%d", x, y)
			}
		}
	}
}

func TestThumbnailKeepsNativeDetailAndSamplesBothHalves(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 512, 512))
	for y := range 512 {
		for x := range 512 {
			c := color.NRGBA{R: 255, A: 255}
			if y >= 256 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	thumbnail, err := NewThumbnail(img)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(thumbnail.png))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 256 || decoded.Bounds().Dy() != 256 {
		t.Fatalf("native detail lost or unbounded: %v", decoded.Bounds())
	}
	// 单独使用两行源像素，精确区分上下半块采样而不依赖缩放滤波细节。
	small := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for x := range 2 {
		small.SetNRGBA(x, 0, color.NRGBA{R: 255, A: 255})
		small.SetNRGBA(x, 1, color.NRGBA{B: 255, A: 255})
	}
	thumbnail, err = NewThumbnail(small)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Repeat("\x1b[38;2;255;0;0m\x1b[48;2;0;0;255m▀", 2) + "\x1b[0m"
	if got := thumbnail.Fallback(2, 1); got != want {
		t.Fatalf("incorrect half-block sampling: %q", got)
	}
}

func TestCellGeometryReservesSquarePixels(t *testing.T) {
	if got := CellGeometry(3, 9, 20); got != 7 {
		t.Fatalf("fractional column clipped square: %d", got)
	}
	if got := CellGeometry(3, 0, 20); got != 6 {
		t.Fatalf("unknown cell size must use fallback aspect: %d", got)
	}
	if got := CellGeometry(0, 9, 20); got != 0 {
		t.Fatalf("empty placement reserved columns: %d", got)
	}
}

func TestPlacementReplacesOnlyPlacementsAndCorrelatesACK(t *testing.T) {
	wire := Place(42, 7, 4, 2, 3)
	want := "\x1b_Ga=d,d=i,i=42,q=2;\x1b\\\x1b7\x1b[3;5H\x1b_Ga=p,i=42,p=7,r=3,C=1,q=0;\x1b\\\x1b8"
	if wire != want {
		t.Fatalf("unsafe or uncorrelated placement: %q", wire)
	}
	if got := Delete(42); got != "\x1b_Ga=d,d=I,i=42,q=2;\x1b\\" {
		t.Fatalf("image data not targeted for release: %q", got)
	}
	if got := Place(42, 8, -1, 2, 3); got != "" {
		t.Fatalf("invalid geometry emitted destructive commands: %q", got)
	}
}
