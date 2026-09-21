package store

import (
	"arcana-world/internal/i18n"

	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"arcana-world/internal/domain"
	"arcana-world/internal/overlay"
	"arcana-world/internal/tts"
	"github.com/zalando/go-keyring"
)

// Backend 存储当前作用域内的机密；条目不存在时必须返回 keyring.ErrNotFound。
type Backend interface {
	Get(key string) (string, error)
	Set(key, value string) error
	Delete(key string) error
	Storage() StorageKind
}
type systemBackend struct {
	service string
}

func newSystemBackend(dir string) Backend {
	sum := sha256.Sum256([]byte(dir))
	return systemBackend{service: "arcana-world/" + hex.EncodeToString(sum[:16])}
}

func (b systemBackend) Get(key string) (string, error) { return keyring.Get(b.service, key) }
func (b systemBackend) Set(key, value string) error    { return keyring.Set(b.service, key, value) }
func (b systemBackend) Delete(key string) error        { return keyring.Delete(b.service, key) }
func (systemBackend) Storage() StorageKind             { return StorageSystem }

type Store struct {
	mu        sync.Mutex
	dir       string
	backend   Backend
	config    domain.Config
	overrides ConfigOverrides
	closed    bool
}

// DefaultConfig 返回新配置和设置重置所用的默认值。
func DefaultConfig() domain.Config {
	return domain.Config{Protocol: "rtmp", OBSURL: "ws://127.0.0.1:4455", OBSAutoConnect: true, OBSAutoStream: true, Overlay: overlay.DefaultSettings(), TTS: tts.DefaultSettings()}
}

func applyConnectionDefaults(c *domain.Config) {
	defaults := DefaultConfig()
	if c.Protocol == "" {
		c.Protocol = defaults.Protocol
	}
	if c.OBSURL == "" {
		c.OBSURL = defaults.OBSURL
	}
}

// Open 使用 auto、system、file 或 memory 凭据存储打开配置。
// 显式指定的后端不会回退，也不会在存储之间迁移凭据。
func Open(dir string, options Options) (*Store, error) {
	switch options.CredentialBackend {
	case "", "auto", "system", "file", "memory":
	default:
		return nil, errors.New("unsupported credential backend")
	}
	s, err := openStore(dir)
	if err != nil {
		return nil, err
	}
	s.overrides = options.Overrides.detached()
	switch options.CredentialBackend {
	case "system":
		s.backend = newSystemBackend(s.dir)
	case "file":
		s.backend = newFileBackend(s.dir)
	case "memory":
		s.backend = memoryBackend(make(map[string]string))
	default:
		s.backend = &automaticBackend{dir: s.dir, system: newSystemBackend(s.dir), probe: systemKeyringMissing}
	}
	return s, nil
}
func OpenWithBackend(dir string, backend Backend) (*Store, error) {
	if backend == nil {
		return nil, errors.New(i18n.T(i18n.StoreCredentialBackendRequired))
	}
	s, err := openStore(dir)
	if err != nil {
		return nil, err
	}
	s.backend = backend
	return s, nil
}

// ResolveDir 解析配置路径，不访问或创建其中的文件。
func ResolveDir(dir string) (string, error) {
	if dir == "" {
		base, err := os.UserHomeDir()
		if err != nil {
			return "", errors.New(i18n.T(i18n.StoreHomeDirectoryUnavailable))
		}
		dir = filepath.Join(base, ".arcana", "world")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", errors.New(i18n.T(i18n.StoreConfigDirectoryResolveFailed))
	}
	return dir, nil
}

func openStore(dir string) (*Store, error) {
	dir, err := ResolveDir(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.New(i18n.T(i18n.StoreConfigDirectoryCreateFailed))
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New(i18n.T(i18n.StoreConfigDirectoryRequired))
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return nil, errors.New(i18n.T(i18n.StoreConfigDirectorySecureFailed))
	}
	c, missing, err := readConfig(dir, true)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, config: c}
	if missing {
		if err := s.persist(c); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// ReadConfig 加载并校验设置，不创建文件、更改权限，
// 也不访问凭据存储。配置不存在时使用默认值。
func ReadConfig(dir string) (domain.Config, error) {
	dir, err := ResolveDir(dir)
	if err != nil {
		return domain.Config{}, err
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return domain.Config{}, errors.New(i18n.T(i18n.StoreConfigDirectoryRequired))
	}
	c, _, err := readConfig(dir, false)
	return c, err
}

func readConfig(dir string, secure bool) (domain.Config, bool, error) {
	c := DefaultConfig()
	path := filepath.Join(dir, "config.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, true, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return domain.Config{}, false, errors.New(i18n.T(i18n.StoreConfigRegularFileRequired))
	}
	if secure {
		if err := os.Chmod(path, 0600); err != nil {
			return domain.Config{}, false, errors.New(i18n.T(i18n.StoreConfigFileSecureFailed))
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return domain.Config{}, false, errors.New(i18n.T(i18n.StoreConfigReadFailed))
	}
	if err = json.Unmarshal(data, &c); err != nil {
		return domain.Config{}, false, errors.New(i18n.T(i18n.StoreConfigJsonInvalid))
	}
	if c.Overlay, err = c.Overlay.Normalize(); err != nil {
		return domain.Config{}, false, err
	}
	if c.TTS, err = c.TTS.Normalize(); err != nil {
		return domain.Config{}, false, err
	}
	applyConnectionDefaults(&c)
	seen := make(map[string]bool, len(c.Accounts))
	for _, a := range c.Accounts {
		if strings.TrimSpace(a.UID) == "" || seen[a.UID] {
			return domain.Config{}, false, errors.New(i18n.T(i18n.StoreAccountIndexInvalid))
		}
		seen[a.UID] = true
	}
	return c, false, nil
}

func (s *Store) Dir() string { return s.dir }
func clone(c domain.Config) domain.Config {
	c.Accounts = append([]domain.AccountInfo(nil), c.Accounts...)
	c.RecentTitles = append([]string(nil), c.RecentTitles...)
	c.RecentAreas = append([]domain.Area(nil), c.RecentAreas...)
	c.OverlayDisabledEvents = append([]string(nil), c.OverlayDisabledEvents...)
	if c.Overlay.Displays != nil {
		c.Overlay.Displays = append([]string{}, c.Overlay.Displays...)
	}
	c.TTS.DisabledEvents = append([]string(nil), c.TTS.DisabledEvents...)
	return c
}
func (s *Store) Config() domain.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overrides.Apply(s.config)
}
func (s *Store) Accounts() []domain.AccountInfo { return s.Config().Accounts }

// SaveConfig 保留由凭据存储支持的账号索引；请使用 Save/Delete 修改索引。
func (s *Store) SaveConfig(c domain.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New(i18n.T(i18n.StoreCleared))
	}
	c.Accounts = s.config.Accounts
	c = clone(c)
	if s.overrides.Proxy != nil {
		c.Proxy = s.config.Proxy
	}
	if s.overrides.OBSAutoConnect != nil {
		c.OBSAutoConnect = s.config.OBSAutoConnect
	}
	if s.overrides.OBSAutoStream != nil {
		c.OBSAutoStream = s.config.OBSAutoStream
	}
	applyConnectionDefaults(&c)
	var err error
	if c.TTS, err = c.TTS.Normalize(); err != nil {
		return err
	}
	if err := s.persist(c); err != nil {
		return err
	}
	s.config = c
	return nil
}
func (s *Store) Load(uid string) (domain.Account, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var info *domain.AccountInfo
	for i := range s.config.Accounts {
		if s.config.Accounts[i].UID == uid {
			info = &s.config.Accounts[i]
			break
		}
	}
	if info == nil {
		return domain.Account{}, errors.New(i18n.T(i18n.StoreAccountNotSaved))
	}
	raw, err := s.backend.Get("account:" + uid)
	if err != nil {
		return domain.Account{}, secretError(i18n.T(i18n.StoreReadAccountCredentials), err)
	}
	var cookies map[string]string
	if err = json.Unmarshal([]byte(raw), &cookies); err != nil || len(cookies) == 0 {
		return domain.Account{}, errors.New(i18n.T(i18n.StoreAccountCredentialsInvalid))
	}
	return domain.Account{UID: uid, Name: info.Name, Cookies: cookies}, nil
}
func (s *Store) Save(a domain.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New(i18n.T(i18n.StoreCleared))
	}
	if strings.TrimSpace(a.UID) == "" || len(a.Cookies) == 0 {
		return errors.New(i18n.T(i18n.StoreAccountCredentialsRequired))
	}
	data, err := json.Marshal(a.Cookies)
	if err != nil {
		return errors.New(i18n.T(i18n.StoreAccountCredentialsEncodeFailed))
	}
	user := "account:" + a.UID
	old, err := s.backend.Get(user)
	exists := err == nil
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return secretError(i18n.T(i18n.StoreReadExistingCredentials), err)
	}
	c := clone(s.config)
	found := false
	for i := range c.Accounts {
		if c.Accounts[i].UID == a.UID {
			c.Accounts[i].Name = a.Name
			found = true
			break
		}
	}
	if !found {
		c.Accounts = append(c.Accounts, domain.AccountInfo{UID: a.UID, Name: a.Name})
	}
	return s.commitCredentialsLocked(c, user, old, exists, func() error {
		if err := s.backend.Set(user, string(data)); err != nil {
			return secretError(i18n.T(i18n.StoreSaveAccountCredentials), err)
		}
		return nil
	})
}
func (s *Store) Delete(uid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New(i18n.T(i18n.StoreCleared))
	}
	c := clone(s.config)
	index := -1
	for i, a := range c.Accounts {
		if a.UID == uid {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New(i18n.T(i18n.StoreAccountNotSaved))
	}
	user := "account:" + uid
	old, err := s.backend.Get(user)
	exists := err == nil
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return secretError(i18n.T(i18n.StoreReadCredentialsBeforeDeletion), err)
	}
	var change func() error
	if exists {
		change = func() error {
			if err := s.backend.Delete(user); err != nil {
				return secretError(i18n.T(i18n.StoreDeleteAccountCredentials), err)
			}
			return nil
		}
	}
	c.Accounts = append(c.Accounts[:index], c.Accounts[index+1:]...)
	if c.ActiveUID == uid {
		c.ActiveUID = ""
	}
	return s.commitCredentialsLocked(c, user, old, exists, change)
}

// commitCredentialsLocked 在凭据变更和索引持久化都成功后发布配置。
// change 为 nil 表示凭据原本不存在，不执行变更或补偿。
func (s *Store) commitCredentialsLocked(c domain.Config, user, old string, exists bool, change func() error) error {
	if change != nil {
		if err := change(); err != nil {
			return err
		}
	}
	if err := s.persist(c); err != nil {
		if change != nil {
			return s.rollback(user, old, exists, err)
		}
		return err
	}
	s.config = c
	return nil
}
func (s *Store) rollback(user, old string, exists bool, cause error) error {
	var err error
	if exists {
		err = s.backend.Set(user, old)
	} else {
		err = s.backend.Delete(user)
		if errors.Is(err, keyring.ErrNotFound) {
			err = nil
		}
	}
	if err != nil {
		return fmt.Errorf(i18n.T(i18n.StoreCredentialRollbackFailed), cause)
	}
	return cause
}
func (s *Store) OBSSecret() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.backend.Get("obs-password")
	if err != nil {
		return "", secretError(i18n.T(i18n.StoreReadObsPassword), err)
	}
	return v, nil
}
func (s *Store) SetOBSSecret(password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New(i18n.T(i18n.StoreCleared))
	}
	if err := s.backend.Set("obs-password", password); err != nil {
		return secretError(i18n.T(i18n.StoreSaveObsPassword), err)
	}
	return nil
}

// 重命名是提交点：此前发生的错误不会影响原有索引。
func (s *Store) persist(c domain.Config) error {
	if s.closed {
		return errors.New(i18n.T(i18n.StoreCleared))
	}
	if _, err := c.Overlay.Normalize(); err != nil {
		return err
	}
	if _, err := c.TTS.Normalize(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return errors.New(i18n.T(i18n.StoreConfigEncodeFailed))
	}
	f, err := os.CreateTemp(s.dir, ".config-*")
	if err != nil {
		return errors.New(i18n.T(i18n.StoreConfigTemporaryFileFailed))
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(append(data, '\n'))
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return errors.New(i18n.T(i18n.StoreConfigWriteFailed))
	}
	if err = os.Rename(name, filepath.Join(s.dir, "config.json")); err != nil {
		return errors.New(i18n.T(i18n.StoreConfigReplaceFailed))
	}
	return nil
}
