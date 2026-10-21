package models

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// 固定 bzip2 测试归档避免依赖外部压缩程序。
func TestExtractPinnedTarBzip2(t *testing.T) {
	sum := sha256.Sum256([]byte("pinned model"))
	spec := Spec{extension: ".tar.bz2", prefix: "bundle", assets: map[string]asset{"model.onnx": {12, hex.EncodeToString(sum[:])}}}
	for _, tc := range []struct {
		name, archive string
		want          error
	}{
		{"valid", "QlpoOTFBWSZTWZ7N8TgAAG99gMqAAQBAAfcAAAD2J95ACAggAHUNChoNNND1AGJ6gklDRpoADQAfewJEIIzIQi7q940HzoEMDGr42uE5hGaEE1iqPPY3FRXyEsjiJ2BM+vnTvSg9alrM1ykRA/F3JFOFCQns3xOA", nil},
		{"traversal", "QlpoOTFBWSZTWbN7LcsAAHD9gMqAAQBAAe0AAAR2J95ACAggAHUNCjQAabUAzU9QSUTRpoMgNAB97YNQgm5CEUdSeMg6lAhgMNYxhNsn4sRWYIKhBIyzBhYE2+UxPU9hSASMvrKXow4drXd7lSQfi7kinChIWb2W5YA=", ErrArchiveInvalid},
		{"symlink", "QlpoOTFBWSZTWdsJP8AAAG3/gMiAABBAAfWAAACAIDYnnkAACCAAVEFBtR6g9T0gaZlBJIjRoaaAAaUps4SC6pCET8naRIQQIYGOYvE9hGjkTvZUwnKRA7Utaqk/F1IM4qr7noUQiMIiAfi7kinChIbYSf4A", ErrArchiveInvalid},
		{"wrongRoot", "QlpoOTFBWSZTWQk1DTYAAHl7gMqAAQBAAecAQABmZ95ACAggAHUNKZPUNqGhoBtE9QSSho9TQAAAfexJEINZkIRb1c40XnQIYGM3osgKDCMXENg+KPkDOwV6ULEenS4R8vWDqCc2qjNBkkRED8XckU4UJAJNQ02A", ErrArchiveInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := base64.StdEncoding.DecodeString(tc.archive)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			filename := filepath.Join(dir, "archive.tar.bz2")
			if err = os.WriteFile(filename, data, 0600); err != nil {
				t.Fatal(err)
			}
			file, err := os.Open(filename)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err = root.Mkdir("staging", 0700); err != nil {
				t.Fatal(err)
			}
			if err = extract(context.Background(), root, "staging", file, int64(len(data)), spec); !errors.Is(err, tc.want) {
				t.Fatalf("extract = %v, want %v", err, tc.want)
			}
			if tc.want == nil {
				got, err := os.ReadFile(filepath.Join(dir, "staging", "model.onnx"))
				if err != nil || string(got) != "pinned model" {
					t.Fatalf("installed model = %q, %v", got, err)
				}
			}
			if _, err := os.Lstat(filepath.Join(dir, "model.onnx")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("archive escaped staging: %v", err)
			}
		})
	}
}
