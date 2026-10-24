package models

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractPinnedZIP(t *testing.T) {
	data := []byte("pinned model")
	sum := sha256.Sum256(data)
	spec := Spec{extension: ".zip", assets: map[string]asset{"model.onnx": {int64(len(data)), hex.EncodeToString(sum[:])}}}
	for _, tc := range []struct {
		name, entry string
		duplicate   bool
		mode        os.FileMode
		want        error
	}{
		{"valid", "model.onnx", false, 0600, nil},
		{"traversal", "../model.onnx", false, 0600, ErrArchiveInvalid},
		{"unknown", "other.onnx", false, 0600, ErrArchiveInvalid},
		{"duplicate", "model.onnx", true, 0600, ErrArchiveInvalid},
		{"symlink", "model.onnx", false, os.ModeSymlink | 0600, ErrArchiveInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			file, err := os.Create(filepath.Join(dir, "archive.zip"))
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			zw := zip.NewWriter(file)
			count := 1
			if tc.duplicate {
				count++
			}
			for range count {
				h := &zip.FileHeader{Name: tc.entry, Method: zip.Store}
				h.SetMode(tc.mode)
				w, err := zw.CreateHeader(h)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = w.Write(data); err != nil {
					t.Fatal(err)
				}
			}
			if err = zw.Close(); err != nil {
				t.Fatal(err)
			}
			info, err := file.Stat()
			if err != nil {
				t.Fatal(err)
			}
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer root.Close()
			if err = root.Mkdir("staging", 0700); err != nil {
				t.Fatal(err)
			}
			if err = extract(context.Background(), root, "staging", file, info.Size(), spec); !errors.Is(err, tc.want) {
				t.Fatalf("extract = %v, want %v", err, tc.want)
			}
			if tc.want == nil {
				got, err := os.ReadFile(filepath.Join(dir, "staging", "model.onnx"))
				if err != nil || string(got) != string(data) {
					t.Fatalf("installed model = %q, %v", got, err)
				}
			}
			if _, err := os.Lstat(filepath.Join(dir, "model.onnx")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("archive escaped staging: %v", err)
			}
		})
	}
}
