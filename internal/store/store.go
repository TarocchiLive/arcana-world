package store

import (
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
	"github.com/zalando/go-keyring"
)

// Backend stores secrets; missing entries must return keyring.ErrNotFound.
type Backend interface {
	Get(service, user string) (string, error)
	Set(service, user, password string) error
	Delete(service, user string) error
}
type systemBackend struct{}

func (systemBackend) Get(s, u string) (string, error) { return keyring.Get(s, u) }
func (systemBackend) Set(s, u, p string) error        { return keyring.Set(s, u, p) }
func (systemBackend) Delete(s, u string) error        { return keyring.Delete(s, u) }

type Store struct {
	mu      sync.Mutex
	dir     string
	service string
	backend Backend
	config  domain.Config
}

func Open(dir string) (*Store, error) { return OpenWithBackend(dir, systemBackend{}) }
func OpenWithBackend(dir string, backend Backend) (*Store, error) {
	if backend == nil {
		return nil, errors.New("credential backend is required")
	}
	if dir == "" {
		base, err := os.UserHomeDir()
		if err != nil {
			return nil, errors.New("cannot locate user home directory")
		}
		dir = filepath.Join(base, ".arcana", "world")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, errors.New("cannot resolve configuration directory")
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.New("cannot create configuration directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("configuration path must be a directory, not a symlink")
	}
	if err = os.Chmod(dir, 0700); err != nil {
		return nil, errors.New("cannot secure configuration directory")
	}
	sum := sha256.Sum256([]byte(dir))
	s := &Store{dir: dir, service: "arcana-world/" + hex.EncodeToString(sum[:16]), backend: backend, config: domain.Config{Protocol: "rtmp", OBSURL: "ws://127.0.0.1:4455"}}
	path := filepath.Join(dir, "config.json")
	info, err = os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		if err = s.persist(s.config); err != nil {
			return nil, err
		}
		return s, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, errors.New("configuration must be a regular file")
	}
	if err = os.Chmod(path, 0600); err != nil {
		return nil, errors.New("cannot secure configuration file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("cannot read configuration")
	}
	if err = json.Unmarshal(data, &s.config); err != nil {
		return nil, errors.New("invalid configuration JSON")
	}
	if s.config.Protocol == "" {
		s.config.Protocol = "rtmp"
	}
	if s.config.OBSURL == "" {
		s.config.OBSURL = "ws://127.0.0.1:4455"
	}
	seen := make(map[string]bool, len(s.config.Accounts))
	for _, a := range s.config.Accounts {
		if strings.TrimSpace(a.UID) == "" || seen[a.UID] {
			return nil, errors.New("invalid account index")
		}
		seen[a.UID] = true
	}
	return s, nil
}

func (s *Store) Dir() string { return s.dir }
func clone(c domain.Config) domain.Config {
	c.Accounts = append([]domain.AccountInfo(nil), c.Accounts...)
	c.RecentTitles = append([]string(nil), c.RecentTitles...)
	c.RecentAreas = append([]domain.Area(nil), c.RecentAreas...)
	return c
}
func (s *Store) Config() domain.Config          { s.mu.Lock(); defer s.mu.Unlock(); return clone(s.config) }
func (s *Store) Accounts() []domain.AccountInfo { return s.Config().Accounts }

// SaveConfig preserves the credential-backed account index. Use Save/Delete to change it.
func (s *Store) SaveConfig(c domain.Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c = clone(c)
	c.Accounts = append([]domain.AccountInfo(nil), s.config.Accounts...)
	if c.Protocol == "" {
		c.Protocol = "rtmp"
	}
	if c.OBSURL == "" {
		c.OBSURL = "ws://127.0.0.1:4455"
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
		return domain.Account{}, errors.New("account is not in the saved index")
	}
	raw, err := s.backend.Get(s.service, "account:"+uid)
	if err != nil {
		return domain.Account{}, secretError("read account credentials", err)
	}
	var cookies map[string]string
	if err = json.Unmarshal([]byte(raw), &cookies); err != nil || len(cookies) == 0 {
		return domain.Account{}, errors.New("saved account credentials are invalid")
	}
	return domain.Account{UID: uid, Name: info.Name, Cookies: cookies}, nil
}
func (s *Store) Save(a domain.Account) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(a.UID) == "" || len(a.Cookies) == 0 {
		return errors.New("account ID and cookies are required")
	}
	data, err := json.Marshal(a.Cookies)
	if err != nil {
		return errors.New("cannot encode account credentials")
	}
	user := "account:" + a.UID
	old, err := s.backend.Get(s.service, user)
	exists := err == nil
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return secretError("read existing account credentials", err)
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
	if err = s.backend.Set(s.service, user, string(data)); err != nil {
		return secretError("save account credentials", err)
	}
	if err = s.persist(c); err != nil {
		return s.rollback(user, old, exists, err)
	}
	s.config = c
	return nil
}
func (s *Store) Delete(uid string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := clone(s.config)
	index := -1
	for i, a := range c.Accounts {
		if a.UID == uid {
			index = i
			break
		}
	}
	if index < 0 {
		return errors.New("account is not in the saved index")
	}
	user := "account:" + uid
	old, err := s.backend.Get(s.service, user)
	exists := err == nil
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return secretError("read account credentials before deletion", err)
	}
	if exists {
		if err = s.backend.Delete(s.service, user); err != nil {
			return secretError("delete account credentials", err)
		}
	}
	c.Accounts = append(c.Accounts[:index], c.Accounts[index+1:]...)
	if c.ActiveUID == uid {
		c.ActiveUID = ""
	}
	if err = s.persist(c); err != nil {
		if exists {
			return s.rollback(user, old, true, err)
		}
		return err
	}
	s.config = c
	return nil
}
func (s *Store) rollback(user, old string, exists bool, cause error) error {
	var err error
	if exists {
		err = s.backend.Set(s.service, user, old)
	} else {
		err = s.backend.Delete(s.service, user)
		if errors.Is(err, keyring.ErrNotFound) {
			err = nil
		}
	}
	if err != nil {
		return fmt.Errorf("%w; credential rollback failed: the credential store needs attention", cause)
	}
	return cause
}
func (s *Store) OBSSecret() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.backend.Get(s.service, "obs-password")
	if err != nil {
		return "", secretError("read OBS password", err)
	}
	return v, nil
}
func (s *Store) SetOBSSecret(password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.backend.Set(s.service, "obs-password", password); err != nil {
		return secretError("save OBS password", err)
	}
	return nil
}
func secretError(action string, err error) error {
	if errors.Is(err, keyring.ErrNotFound) {
		return fmt.Errorf("cannot %s: %w", action, keyring.ErrNotFound)
	}
	return fmt.Errorf("cannot %s: credential store unavailable or access denied", action)
}

// Rename is the commit point: errors before it leave the previous index intact.
func (s *Store) persist(c domain.Config) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return errors.New("cannot encode configuration")
	}
	f, err := os.CreateTemp(s.dir, ".config-*")
	if err != nil {
		return errors.New("cannot create temporary configuration")
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
		return errors.New("cannot write configuration")
	}
	if err = os.Rename(name, filepath.Join(s.dir, "config.json")); err != nil {
		return errors.New("cannot replace configuration")
	}
	return nil
}
