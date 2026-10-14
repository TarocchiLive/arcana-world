package coverimage

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/jpeg"
	"image/png"
	"math/rand/v2"
	"strings"
	"testing"
)

// 大体积 PNG 必须压缩到上传上限；伪造的巨大 IHDR 必须在像素解码前拒绝。
func TestPrepareUploadBoundary(t *testing.T) {
	var encoded bytes.Buffer
	source := image.NewNRGBA(image.Rect(0, 0, Width, Height))
	random := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < len(source.Pix); i += 4 {
		value := random.Uint64()
		source.Pix[i], source.Pix[i+1], source.Pix[i+2], source.Pix[i+3] = byte(value), byte(value>>8), byte(value>>16), 255
	}
	if err := png.Encode(&encoded, source); err != nil {
		t.Fatal(err)
	}
	raw := encoded.Bytes()
	if len(raw) <= MaxUploadSize {
		t.Fatal("regression fixture must exceed the upload limit before preparation")
	}
	const prefix = "data:image/png;base64,"
	valid := prefix + base64.StdEncoding.EncodeToString(raw)
	prepared, err := PrepareUpload(context.Background(), valid)
	if err != nil {
		t.Fatal(err)
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(prepared.JPEG))
	if err != nil || config.Width != Width || config.Height != Height || len(prepared.JPEG) > MaxUploadSize {
		t.Fatalf("prepared cover must be a bounded JPEG: config=%+v bytes=%d err=%v", config, len(prepared.JPEG), err)
	}
	oversized := bytes.Clone(raw)
	binary.BigEndian.PutUint32(oversized[16:20], 100_000)
	binary.BigEndian.PutUint32(oversized[20:24], 100_000)
	binary.BigEndian.PutUint32(oversized[29:33], crc32.ChecksumIEEE(oversized[12:29]))
	for _, tc := range []struct{ name, value string }{
		{"wrong media type", strings.Replace(valid, "image/png", "image/jpeg", 1)},
		{"encoded bound", prefix + strings.Repeat("A", 2<<20)},
		{"invalid base64", prefix + "@@@@"},
		{"base64 newline", valid + "\n"},
		{"noncanonical base64", prefix + "AB=="},
		{"wrong dimensions", prefix + base64.StdEncoding.EncodeToString(oversized)},
		{"truncated image", prefix + base64.StdEncoding.EncodeToString(raw[:33])},
		{"trailing data", prefix + base64.StdEncoding.EncodeToString(append(bytes.Clone(raw), 0))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := PrepareUpload(context.Background(), tc.value); err == nil {
				t.Fatal("invalid upload accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := PrepareUpload(ctx, valid); err != context.Canceled {
		t.Fatalf("canceled preparation: %v", err)
	}
}
