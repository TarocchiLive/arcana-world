// Package models 管理固定版本的本地语音资源，不读取或传递账号凭据。
package models

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"arcana-world/internal/i18n"

	bolt "go.etcd.io/bbolt"
)

type modelError uint8

const (
	ErrMissing modelError = iota
	ErrIntegrity
	ErrBusy
	ErrDownload
	ErrUnsafePath
	ErrArchiveInvalid
)

func (e modelError) Error() string {
	keys := [...]i18n.Key{i18n.TTSModelMissing, i18n.TTSModelIntegrity, i18n.TTSModelBusy, i18n.TTSModelDownloadFailed, i18n.TTSModelUnsafePath, i18n.TTSModelArchiveInvalid}
	return i18n.T(keys[e])
}

type Progress struct {
	Downloaded, Total int64
	Stage             string
}
type Status struct {
	Dir, Archive string
	Installed    bool
}

// DefaultDir 返回应用目录下供所有用户配置共享的固定模型目录。
func DefaultDir(appDir, provider string) string {
	spec, ok := SpecFor(provider)
	if !ok {
		return ""
	}
	return filepath.Join(appDir, "models", "tts", spec.Name)
}
func ArchivePath(appDir, provider string) string {
	spec, ok := SpecFor(provider)
	if !ok {
		return ""
	}
	return filepath.Join(appDir, "models", "tts", spec.Name+spec.extension)
}

// Inspect 不做大文件散列；Installed 仅表示资源完整存在，使用前仍须 Verify。
func Inspect(appDir, provider string) Status {
	spec, ok := SpecFor(provider)
	if !ok {
		return Status{}
	}
	assets := spec.assets
	s := Status{Dir: DefaultDir(appDir, provider), Archive: ArchivePath(appDir, provider)}
	r, err := os.OpenRoot(s.Dir)
	if err != nil {
		return s
	}
	defer r.Close()
	for name, asset := range assets {
		info, err := r.Lstat(filepath.FromSlash(name))
		if err != nil || !info.Mode().IsRegular() || info.Size() != asset.size {
			return s
		}
	}
	s.Installed = true
	return s
}

type contextReader struct {
	ctx context.Context
	io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.Reader.Read(p)
}

func digest(ctx context.Context, r io.Reader) (string, error) {
	h := sha256.New()
	if _, err := io.Copy(h, contextReader{ctx, r}); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Verify 校验固定资产并拒绝链接；普通附加文件不参与推理，不执行包内 config.json。
func Verify(ctx context.Context, dir, provider string) error {
	spec, ok := SpecFor(provider)
	if !ok {
		return ErrMissing
	}
	assets := spec.assets
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return ErrMissing
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return ErrUnsafePath
	}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return ErrMissing
	}
	defer r.Close()
	seen := 0
	err = fs.WalkDir(r.FS(), ".", func(name string, d fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return ErrMissing
		}
		if d.Type()&os.ModeSymlink != 0 {
			return ErrUnsafePath
		}
		if d.IsDir() {
			return nil
		}
		expected, ok := assets[name]
		if !ok {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return ErrUnsafePath
		}
		if info.Size() != expected.size {
			return ErrIntegrity
		}
		file, err := r.Open(filepath.FromSlash(name))
		if err != nil {
			return ErrMissing
		}
		hash, err := digest(ctx, file)
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
		if hash != expected.sha256 {
			return ErrIntegrity
		}
		seen++
		return nil
	})
	if err != nil {
		return err
	}
	if seen != len(assets) {
		return ErrMissing
	}
	return nil
}

var installations struct {
	sync.Mutex
	active map[string]bool
}

func reserve(dir string) (func(), error) {
	installations.Lock()
	defer installations.Unlock()
	if installations.active == nil {
		installations.active = make(map[string]bool)
	}
	if installations.active[dir] {
		return nil, ErrBusy
	}
	installations.active[dir] = true
	return func() { installations.Lock(); delete(installations.active, dir); installations.Unlock() }, nil
}

func modelRoot(appDir string) (*os.Root, error) {
	r, err := os.OpenRoot(appDir)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	for _, name := range []string{"models", filepath.Join("models", "tts")} {
		info, err := r.Lstat(name)
		if errors.Is(err, os.ErrNotExist) {
			if err = r.Mkdir(name, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				return nil, err
			}
			info, err = r.Lstat(name)
		}
		if err != nil {
			return nil, err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrUnsafePath
		}
	}
	return r.OpenRoot(filepath.Join("models", "tts"))
}
func regular(r *os.Root, name string) error {
	info, err := r.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return ErrUnsafePath
	}
	return nil
}
func notify(progress func(Progress), spec Spec, stage string, n int64) {
	if progress != nil {
		progress(Progress{Downloaded: n, Total: spec.ArchiveSize, Stage: stage})
	}
}

// Install 优先采用用户放入的压缩包；网络中断留下 .part，完整校验后才替换模型目录。
func Install(ctx context.Context, appDir, proxy, provider string, progress func(Progress)) error {
	spec, ok := SpecFor(provider)
	if !ok {
		return ErrMissing
	}
	modelName := spec.Name
	if err := ctx.Err(); err != nil {
		return err
	}
	absolute, err := filepath.Abs(appDir)
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
		absolute = resolved
	}
	release, err := reserve(filepath.Join(absolute, spec.Name))
	if err != nil {
		return err
	}
	defer release()
	r, err := modelRoot(absolute)
	if err != nil {
		return err
	}
	defer r.Close()
	// 与日志锁采用相同的 Bolt 跨平台咨询锁；稳定的锁文件不能随模型发布或用户数据清理移除。
	lockName := "." + modelName + ".lock"
	if err = regular(r, lockName); err != nil {
		return err
	}
	lock, err := bolt.Open(lockName, 0600, &bolt.Options{
		Timeout: time.Second,
		OpenFile: func(_ string, flag int, perm os.FileMode) (*os.File, error) {
			return r.OpenFile(lockName, flag, perm)
		},
	})
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if errors.Is(err, bolt.ErrTimeout) {
			return ErrBusy
		}
		return err
	}
	defer lock.Close()
	if err = ctx.Err(); err != nil {
		return err
	}
	backup := "." + modelName + ".previous"
	if info, backupErr := r.Lstat(backup); backupErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafePath
		}
		if _, currentErr := r.Lstat(modelName); errors.Is(currentErr, os.ErrNotExist) {
			if err = r.Rename(backup, modelName); err != nil {
				return err
			}
		} else if currentErr != nil {
			return currentErr
		} else if err = r.RemoveAll(backup); err != nil {
			// 当前目录已存在，说明备份属于上一次安装，已无用处；完整性由下面统一校验。
			return err
		}
	} else if !errors.Is(backupErr, os.ErrNotExist) {
		return backupErr
	}
	notify(progress, spec, "verifying", 0)
	if Verify(ctx, DefaultDir(absolute, provider), provider) == nil {
		notify(progress, spec, "ready", spec.ArchiveSize)
		return nil
	}
	archive := modelName + spec.extension
	if err = regular(r, archive); err != nil {
		return err
	}
	if _, err = r.Stat(archive); errors.Is(err, os.ErrNotExist) {
		if err = download(ctx, r, proxy, spec, progress); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	notify(progress, spec, "verifying", spec.ArchiveSize)
	file, err := r.Open(archive)
	if err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return err
	}
	if info.Size() != spec.ArchiveSize {
		file.Close()
		return ErrIntegrity
	}
	hash, err := digest(ctx, file)
	if err != nil {
		file.Close()
		return err
	}
	if hash != spec.archiveSHA256 {
		file.Close()
		return ErrIntegrity
	}
	var nonce [12]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		file.Close()
		return err
	}
	staging := ".install-" + hex.EncodeToString(nonce[:])
	if err = r.Mkdir(staging, 0700); err != nil {
		file.Close()
		return err
	}
	defer r.RemoveAll(staging)
	notify(progress, spec, "extracting", spec.ArchiveSize)
	err = extract(ctx, r, staging, file, info.Size(), spec)
	file.Close()
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	previous := false
	if info, statErr := r.Lstat(modelName); statErr == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrUnsafePath
		}
		if err = r.Rename(modelName, backup); err != nil {
			return err
		}
		previous = true
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err = r.Rename(staging, modelName); err != nil {
		if previous {
			if rollback := r.Rename(backup, modelName); rollback != nil {
				return fmt.Errorf("%w; restore %s: %w", err, backup, rollback)
			}
		}
		return err
	}
	// 新模型已发布，安装不再算失败；残留的旧版本备份会在下次安装时清理。
	// 发布目录后保留完整压缩包供离线重装。
	if previous {
		_ = r.RemoveAll(backup)
	}
	notify(progress, spec, "ready", spec.ArchiveSize)
	return nil
}
