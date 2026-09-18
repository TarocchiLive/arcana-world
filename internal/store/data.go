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

// ResetSettings resets application, overlay and speech preferences, leaving
// credentials, OBS connection settings, account selection and histories untouched.
func (s *Store) ResetSettings() (domain.Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := clone(s.config)
	d := DefaultConfig()
	c.Proxy = d.Proxy
	c.Protocol = d.Protocol
	c.ExitOBSStopDisabled = d.ExitOBSStopDisabled
	c.ExitLiveStopDisabled = d.ExitLiveStopDisabled
	c.Overlay = d.Overlay
	c.OverlayDisabledEvents = d.OverlayDisabledEvents
	c.TTS = d.TTS
	if err := s.persist(c); err != nil {
		return domain.Config{}, err
	}
	s.config = c
	return clone(c), nil
}

// ClearData requires the caller to close speech, overlay, listener/history, OBS,
// and journal first. Once attempted, this Store rejects all further writes,
// even if clearing fails. ClearData itself remains retryable. Credentials are
// deleted before files; any failure preserves the account index for retry.
// Only known application files are removed, never the containing data directory.
func (s *Store) ClearData() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	var failures []error
	deleteAccount := func(uid string) {
		if err := s.backend.Delete(s.service, "account:"+uid); err != nil && !errors.Is(err, keyring.ErrNotFound) {
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
	// A damaged account index may still identify one credential via ActiveUID.
	if unlistedActiveUID != "" {
		deleteAccount(unlistedActiveUID)
	}
	if err := s.backend.Delete(s.service, "obs-password"); err != nil && !errors.Is(err, keyring.ErrNotFound) {
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
	// Root constrains all resolution, including concurrent symlink replacement,
	// to this directory. Removing a leaf symlink removes the link, not its target.
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
			// The application never creates a regular file at a directory path.
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
	// Keep config.json until all other removals succeed, retaining the retry index.
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
