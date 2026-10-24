package store

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"arcana-world/internal/config"
	"arcana-world/internal/i18n"
	"arcana-world/internal/tts"

	"github.com/zalando/go-keyring"
)

// ResetSettings 重置应用、浮层和语音偏好，保留凭据、OBS 连接设置、
// 启动界面偏好、账号选择、屏蔽词和历史记录。
func (s *Store) ResetSettings() (config.Config, error) {
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
	c.DanmakuLimit = d.DanmakuLimit
	c.DanmakuShowOther = d.DanmakuShowOther
	c.Overlay = d.Overlay
	c.OverlayDisabledEvents = d.OverlayDisabledEvents
	c.TTS = d.TTS
	if err := s.persist(c); err != nil {
		logger.Error("settings reset failed", "err", err)
		return config.Config{}, err
	}
	logger.Info("settings reset to defaults")
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
			s.credentialFailed("clear_delete_account", uid, err)
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
		s.credentialFailed("clear_delete_obs_password", "", err)
		failures = append(failures, secretError(i18n.T(i18n.StoreDeleteObsPassword), err))
	}
	for _, provider := range tts.Providers() {
		for _, field := range provider.Fields {
			if !field.Secret {
				continue
			}
			if err := s.backend.Delete(ttsSecretKey(provider.ID, field.ID)); err != nil && !errors.Is(err, keyring.ErrNotFound) {
				s.credentialFailed("clear_delete_tts_secret", "", err)
				failures = append(failures, secretError(i18n.T(i18n.TTSSecretDelete), err))
			}
		}
	}
	if err := errors.Join(failures...); err != nil {
		logger.Error("clear data aborted", "stage", "credentials", "failures", len(failures), "backend", storageName(s.backend.Storage()))
		return err
	}
	if err := s.clearFiles(); err != nil {
		logger.Error("clear data failed", "stage", "files", "dir", s.dir, "err", err)
		return fmt.Errorf(i18n.T(i18n.StoreClearDataFailed), err)
	}
	logger.Info("data cleared", "dir", s.dir, "accounts", len(s.config.Accounts), "backend", storageName(s.backend.Storage()))
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
		{"templates", []string{"overlay.zh-CN.yaml", "overlay.en.yaml", "tts.zh-CN.yaml", "tts.en.yaml"}, ""},
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
	if err := removeOwnedFile(root, deviceFile); err != nil {
		return err
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
		logger.Error("owned file stat failed", "dir", root.Name(), "file", name, "err", err)
		return err
	}
	if info.IsDir() {
		logger.Error("owned file is a directory", "dir", root.Name(), "file", name)
		return errors.New(i18n.T(i18n.StoreConfigRegularFileRequired))
	}
	if err := root.Remove(name); err != nil {
		logger.Error("owned file remove failed", "dir", root.Name(), "file", name, "err", err)
		return err
	}
	logger.Info("owned file removed", "dir", root.Name(), "file", name)
	return nil
}
