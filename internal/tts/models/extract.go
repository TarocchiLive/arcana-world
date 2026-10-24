package models

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// extract 仅解包固定清单，大小、路径、类型与摘要均需匹配；不接受链接或设备。
func extract(ctx context.Context, parent *os.Root, staging string, file *os.File, size int64, spec Spec) error {
	root, err := parent.OpenRoot(staging)
	if err != nil {
		return err
	}
	defer root.Close()
	seen := make(map[string]bool, len(spec.assets))
	entryLimit := len(spec.assets)*2 + 128
	consume := func(raw string, directory bool, size int64, input io.Reader) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		name := strings.TrimSuffix(raw, "/")
		if name == "" || name == "." || path.IsAbs(name) || path.Clean(name) != name || strings.ContainsAny(name, "\\:\x00") || strings.HasPrefix(name, "../") {
			return ErrArchiveInvalid
		}
		if spec.prefix != "" {
			if name == spec.prefix && directory {
				return nil
			}
			if !strings.HasPrefix(name, spec.prefix+"/") {
				return ErrArchiveInvalid
			}
			name = strings.TrimPrefix(name, spec.prefix+"/")
		}
		if directory {
			for assetName := range spec.assets {
				if strings.HasPrefix(assetName, name+"/") {
					return root.MkdirAll(filepath.FromSlash(name), 0700)
				}
			}
			return ErrArchiveInvalid
		}
		expected, ok := spec.assets[name]
		if !ok || seen[name] || size != expected.size {
			return ErrArchiveInvalid
		}
		seen[name] = true
		if dir := path.Dir(name); dir != "." {
			if err := root.MkdirAll(filepath.FromSlash(dir), 0700); err != nil {
				return err
			}
		}
		output, err := root.OpenFile(filepath.FromSlash(name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		hash := sha256.New()
		sink := &failureWriter{w: output}
		n, copyErr := io.Copy(io.MultiWriter(sink, hash), io.LimitReader(contextReader{ctx, input}, expected.size+1))
		syncErr := output.Sync()
		closeErr := output.Close()
		// 磁盘写入失败（如空间不足）与压缩包损坏分开报告，便于用户处理。
		if sink.err != nil {
			return sink.err
		}
		if copyErr != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return ErrArchiveInvalid
		}
		if syncErr != nil {
			return syncErr
		}
		if closeErr != nil {
			return closeErr
		}
		if n != expected.size || hex.EncodeToString(hash.Sum(nil)) != expected.sha256 {
			return ErrIntegrity
		}
		return nil
	}
	switch spec.extension {
	case ".zip":
		archive, err := zip.NewReader(file, size)
		if err != nil || len(archive.File) > entryLimit {
			return ErrArchiveInvalid
		}
		for _, entry := range archive.File {
			if entry.Mode()&os.ModeSymlink != 0 || (!entry.Mode().IsRegular() && !entry.FileInfo().IsDir()) {
				return ErrArchiveInvalid
			}
			input, err := entry.Open()
			if err != nil {
				return ErrArchiveInvalid
			}
			err = consume(entry.Name, entry.FileInfo().IsDir(), int64(entry.UncompressedSize64), input)
			closeErr := input.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return ErrArchiveInvalid
			}
		}
	case ".tar.bz2":
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return err
		}
		// 限制解压后的总字节，连同 tar 元数据也不能无限膨胀。
		var limit int64 = int64(entryLimit) * 4096
		for _, asset := range spec.assets {
			limit += asset.size
		}
		stream := &io.LimitedReader{R: bzip2.NewReader(contextReader{ctx, file}), N: limit + 1}
		archive := tar.NewReader(stream)
		for count := 0; ; count++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			entry, err := archive.Next()
			if err == io.EOF {
				break
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil || count >= entryLimit {
				return ErrArchiveInvalid
			}
			if entry.Typeflag != tar.TypeReg && entry.Typeflag != tar.TypeDir {
				return ErrArchiveInvalid
			}
			if err := consume(entry.Name, entry.Typeflag == tar.TypeDir, entry.Size, archive); err != nil {
				return err
			}
		}
		if _, err := io.Copy(io.Discard, contextReader{ctx, stream}); err != nil {
			return err
		}
		if stream.N <= 0 {
			return ErrArchiveInvalid
		}
	default:
		return ErrArchiveInvalid
	}
	if len(seen) != len(spec.assets) {
		return ErrMissing
	}
	return nil
}

// failureWriter 记录底层写入错误，用来区分磁盘问题与输入数据问题。
type failureWriter struct {
	w   io.Writer
	err error
}

func (f *failureWriter) Write(p []byte) (int, error) {
	n, err := f.w.Write(p)
	if err != nil {
		f.err = err
	}
	return n, err
}
