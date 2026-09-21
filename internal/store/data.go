package store

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"arcana-world/internal/domain"
	"arcana-world/internal/i18n"
	"github.com/zalando/go-keyring"
)

// ResetSettings 重置应用、浮层和语音偏好，保留凭据、OBS 连接设置、
// 账号选择和历史记录。
func (s *Store) ResetSettings() (domain.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := clone(s.config)
	d := DefaultConfig()
	c.Proxy = d.Proxy
	c.Protocol = d.Protocol
	c.TUITheme = d.TUITheme
	c.TUIMotionDisabled = d.TUIMotionDisabled
	c.TUICompactHeader = d.TUICompactHeader
	c.TUINotificationsWarningsOnly = d.TUINotificationsWarningsOnly
	c.ExitOBSStopDisabled = d.ExitOBSStopDisabled
	c.ExitLiveStopDisabled = d.ExitLiveStopDisabled
	c.Overlay = d.Overlay
	c.OverlayDisabledEvents = d.OverlayDisabledEvents
	c.TTS = d.TTS
	if err := s.persist(c); err != nil {
		return domain.Config{}, err
	}
	s.config = c
	return s.overrides.Apply(c), nil
}

// ClearData 要求调用方先关闭语音、浮层、监听器及历史记录、OBS 和日志。
// 一旦尝试清理，此 Store 就会拒绝后续所有写入，
// 即使清理失败也不例外。ClearData 本身仍可重试。先删除凭据，
// 再删除文件；任何失败都会保留账号索引以便重试。
// 仅删除已知的应用文件，绝不删除其所在的数据目录。
func (s *Store) ClearData() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	var failures []error
	deleteAccount := func(uid string) {
		if err := s.backend.Delete("account:" + uid); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			failures = append(failures, secretError(i18n.T(i18n.StoreDeleteAccountCredentials), err))
		}
	}
	unlistedActiveUID := s.config.ActiveUID
	for _, a := range s.config.Accounts {
		deleteAccount(a.UID)
		if a.UID == unlistedActiveUID {
			unlistedActiveUID = ""
		}
	}
	// 损坏的账号索引仍可能通过 ActiveUID 标识一份凭据。
	if unlistedActiveUID != "" {
		deleteAccount(unlistedActiveUID)
	}
	if err := s.backend.Delete("obs-password"); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		failures = append(failures, secretError(i18n.T(i18n.StoreDeleteObsPassword), err))
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	if err := s.clearFiles(); err != nil {
		return fmt.Errorf(i18n.T(i18n.StoreClearDataFailed), err)
	}
	s.config = DefaultConfig()
	return nil
}

func (s *Store) clearFiles() error {
	info, err := os.Lstat(s.dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New(i18n.T(i18n.StoreConfigDirectoryRequired))
	}
	root, err := os.OpenRoot(s.dir)
	if err != nil {
		return err
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil {
		return err
	}
	if !os.SameFile(info, opened) {
		return errors.New(i18n.T(i18n.StoreConfigDirectoryRequired))
	}
	// Root 将所有路径解析限制在此目录内，包括并发替换符号链接的情况。
	// 删除末级符号链接只会删除链接本身，不会删除其目标。
	for _, child := range []struct {
		name   string
		files  []string
		prefix string
	}{
		{"danmaku", []string{"history.db"}, ""},
		{"logs", []string{"arcana-world.log", "journal.lock"}, ".journal-"},
		{"credentials", []string{"secrets.json"}, ".secrets-"},
	} {
		info, err := root.Lstat(child.name)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if err := root.Remove(child.name); err != nil {
				return err
			}
			continue
		}
		if !info.IsDir() {
			// 应用绝不会在应为目录的路径上创建普通文件。
			return errors.New(i18n.T(i18n.StoreConfigDirectoryRequired))
		}
		dir, err := root.OpenRoot(child.name)
		if err != nil {
			return err
		}
		err = removeOwnedFiles(dir, child.files, child.prefix)
		closeErr := dir.Close()
		if err := errors.Join(err, closeErr); err != nil {
			return err
		}
	}
	// 在其他删除操作全部成功前保留 config.json，以保留重试所需的索引。
	if err := removeOwnedFiles(root, nil, ".config-"); err != nil {
		return err
	}
	return removeOwnedFile(root, "config.json")
}

func removeOwnedFiles(root *os.Root, files []string, prefix string) error {
	for _, name := range files {
		if err := removeOwnedFile(root, name); err != nil {
			return err
		}
	}
	if prefix == "" {
		return nil
	}
	dir, err := root.Open(".")
	if err != nil {
		return err
	}
	entries, readErr := dir.ReadDir(-1)
	if err := errors.Join(readErr, dir.Close()); err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), prefix) && !entry.IsDir() {
			if err := removeOwnedFile(root, entry.Name()); err != nil {
				return err
			}
		}
	}
	return nil
}

func removeOwnedFile(root *os.Root, name string) error {
	info, err := root.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.IsDir() {
		return errors.New(i18n.T(i18n.StoreConfigRegularFileRequired))
	}
	return root.Remove(name)
}
