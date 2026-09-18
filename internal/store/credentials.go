package store

import (
	"os"
	"path/filepath"

	"arcana-world/internal/i18n"
)

// automaticBackend 在进程内只选择一次后端；已有凭据文件时继续使用文件，
// 避免重启后因密钥环恢复而无法读取原有凭据。
type automaticBackend struct {
	dir      string
	system   Backend
	selected Backend
	file     bool
	saved    bool
	probe    func() (bool, error)
}

func (b *automaticBackend) backend() (Backend, error) {
	if b.selected != nil {
		return b.selected, nil
	}
	_, err := os.Lstat(filepath.Join(b.dir, "credentials", "secrets.json"))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	missing := err == nil
	if !missing {
		missing, err = b.probe()
		if err != nil {
			return nil, err
		}
	}
	if missing {
		b.selected = newFileBackend(b.dir)
		b.file = true
	} else {
		b.selected = b.system
	}
	return b.selected, nil
}

func (b *automaticBackend) Get(service, user string) (string, error) {
	backend, err := b.backend()
	if err != nil {
		return "", err
	}
	return backend.Get(service, user)
}

func (b *automaticBackend) Set(service, user, value string) error {
	b.saved = false
	backend, err := b.backend()
	if err != nil {
		return err
	}
	if err := backend.Set(service, user, value); err != nil {
		return err
	}
	b.saved = b.file
	return nil
}

func (b *automaticBackend) Delete(service, user string) error {
	backend, err := b.backend()
	if err != nil {
		return err
	}
	return backend.Delete(service, user)
}

// CredentialStorageNotice 返回明文凭据成功写入后的提示。
func (s *Store) CredentialStorageNotice() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if b, ok := s.backend.(*automaticBackend); ok && b.saved {
		return i18n.T(i18n.StoreFileStorageFallback)
	}
	return ""
}
