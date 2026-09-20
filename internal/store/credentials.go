package store

import (
	"os"
	"path/filepath"
)

// StorageKind 标识当前选中的凭据存储类型。
type StorageKind uint8

const (
	StorageUnknown StorageKind = iota
	StorageSystem
	StorageFile
)

// automaticBackend 在进程内只选择一次后端；已有凭据文件时继续使用文件，
// 避免重启后因密钥环恢复而无法读取原有凭据。
type automaticBackend struct {
	dir      string
	system   Backend
	selected Backend
	probe    func() (bool, error)
}

func (b *automaticBackend) backend() (Backend, error) {
	if b.selected != nil {
		return b.selected, nil
	}
	_, err := os.Lstat(filepath.Join(b.dir, "credentials", credentialFile))
	if err == nil {
		b.selected = newFileBackend(b.dir)
		return b.selected, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	missing, err := b.probe()
	if err != nil {
		return nil, err
	}
	if missing {
		b.selected = newFileBackend(b.dir)
	} else {
		b.selected = b.system
	}
	return b.selected, nil
}

func (b *automaticBackend) Get(key string) (string, error) {
	backend, err := b.backend()
	if err != nil {
		return "", err
	}
	return backend.Get(key)
}

func (b *automaticBackend) Set(key, value string) error {
	backend, err := b.backend()
	if err != nil {
		return err
	}
	return backend.Set(key, value)
}

func (b *automaticBackend) Delete(key string) error {
	backend, err := b.backend()
	if err != nil {
		return err
	}
	return backend.Delete(key)
}

func (b *automaticBackend) Storage() StorageKind {
	if b.selected == nil {
		return StorageUnknown
	}
	return b.selected.Storage()
}

// CredentialStorage 返回当前选中的凭据存储类型，不触发后端选择。
func (s *Store) CredentialStorage() StorageKind {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backend.Storage()
}
