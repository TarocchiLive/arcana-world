package store

import (
	"arcana-world/internal/config"
	"arcana-world/internal/i18n"

	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"arcana-world/internal/domain"
	"arcana-world/internal/journal"
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
	deviceID  string
	backend   Backend
	config    config.Config
	overrides ConfigOverrides
	closed    bool
}

var logger = journal.Logger("store")

// storageName 返回凭据后端的日志名称。
func storageName(kind StorageKind) string {
	switch kind {
	case StorageSystem:
		return "system"
	case StorageFile:
		return "file"
	case StorageMemory:
		return "memory"
	default:
		return "unselected"
	}
}

// credentialFailed 记录凭据读写删除失败的原始错误；条目不存在属于可恢复情况。
func (s *Store) credentialFailed(op, uid string, err error) {
	if errors.Is(err, keyring.ErrNotFound) {
		logger.Debug("credential not found", "op", op, "uid", uid, "backend", storageName(s.backend.Storage()), "err", err)
		return
	}
	logger.Error("credential operation failed", "op", op, "uid", uid, "backend", storageName(s.backend.Storage()), "err", err)
}

// addProxySecret 把代理 URL 中的密码登记为需脱敏的值。
func addProxySecret(proxy string) {
	if proxy == "" {
		return
	}
	u, err := url.Parse(proxy)
	if err != nil || u.User == nil {
		return
	}
	if password, ok := u.User.Password(); ok {
		journal.AddSecrets(password)
	}
}

// addCookieSecrets 把账号的全部 cookie 值登记为需脱敏的值。
func addCookieSecrets(cookies map[string]string) {
	values := make([]string, 0, len(cookies))
	for _, v := range cookies {
		values = append(values, v)
	}
	journal.AddSecrets(values...)
}

// DefaultConfig 返回新配置和设置重置所用的默认值。
func DefaultConfig() config.Config {
	return config.Config{WebBindAddress: "127.0.0.1", Protocol: "rtmp", OBSURL: "ws://127.0.0.1:4455", OBSAutoConnect: true, OBSAutoStream: true, DanmakuLimit: 30, WebSocketAddr: "127.0.0.1:7391", Overlay: overlay.DefaultSettings(), TTS: tts.DefaultSettings()}
}

func applyConfigDefaults(c *config.Config) {
	defaults := DefaultConfig()
	if c.WebBindAddress == "" {
		c.WebBindAddress = defaults.WebBindAddress
	}
	if c.Protocol == "" {
		c.Protocol = defaults.Protocol
	}
	if c.OBSURL == "" {
		c.OBSURL = defaults.OBSURL
	}
	if c.WebSocketAddr == "" {
		c.WebSocketAddr = defaults.WebSocketAddr
	}
	if c.DanmakuLimit < 1 || c.DanmakuLimit > 1000 {
		c.DanmakuLimit = defaults.DanmakuLimit
	}
}

// Open 使用 auto、system、file 或 memory 凭据存储打开配置。
// 显式指定的后端不会回退，也不会在存储之间迁移凭据。
func Open(dir string, options Options) (*Store, error) {
	switch options.CredentialBackend {
	case "", "auto", "system", "file", "memory":
	default:
		logger.Error("unsupported credential backend", "requested", options.CredentialBackend)
		return nil, errors.New("unsupported credential backend")
	}
	s, err := openStore(dir)
	if err != nil {
		return nil, err
	}
	s.overrides = options.Overrides.detached()
	if s.overrides.Proxy != nil {
		addProxySecret(*s.overrides.Proxy)
	}
	requested := options.CredentialBackend
	if requested == "" {
		requested = "auto"
	}
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
	logger.Info("store opened", "dir", s.dir, "requested_backend", requested, "backend", storageName(s.backend.Storage()),
		"accounts", len(s.config.Accounts), "proxy_override", s.overrides.Proxy != nil)
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
			logger.Error("home directory unavailable", "err", err)
			return "", errors.New(i18n.T(i18n.StoreHomeDirectoryUnavailable))
		}
		dir = filepath.Join(base, ".arcana", "world")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		logger.Error("config directory resolve failed", "dir", dir, "err", err)
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
		logger.Error("config directory create failed", "dir", dir, "err", err)
		return nil, errors.New(i18n.T(i18n.StoreConfigDirectoryCreateFailed))
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		logger.Error("config directory invalid", "dir", dir, "err", err)
		return nil, errors.New(i18n.T(i18n.StoreConfigDirectoryRequired))
	}
	if err = os.Chmod(dir, 0700); err != nil {
		logger.Error("config directory chmod failed", "dir", dir, "err", err)
		return nil, errors.New(i18n.T(i18n.StoreConfigDirectorySecureFailed))
	}
	c, data, err := readConfig(dir, true)
	if err != nil {
		return nil, err
	}
	deviceID, err := loadDeviceID(dir)
	if err != nil {
		logger.Error("device id load failed", "dir", dir, "err", err)
		return nil, errors.New(i18n.T(i18n.StoreDeviceIdentifierFailed))
	}
	s := &Store{dir: dir, deviceID: deviceID, config: c}
	if data == nil {
		logger.Info("config created with defaults", "dir", dir)
		if err := s.persist(c); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// ReadConfig 加载并校验设置，不创建文件、更改权限，
// 也不访问凭据存储。配置不存在时使用默认值。
func ReadConfig(dir string) (config.Config, error) {
	dir, err := ResolveDir(dir)
	if err != nil {
		return config.Config{}, err
	}
	info, err := os.Lstat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		logger.Error("config directory invalid", "dir", dir, "err", err)
		return config.Config{}, errors.New(i18n.T(i18n.StoreConfigDirectoryRequired))
	}
	c, _, err := readConfig(dir, false)
	return c, err
}

// SaveDefaultUI 仅更新启动界面，不读取凭据或创建设备标识。
// 保留原始配置中的其他字段，避免保存偏好时改写账号索引和设置。
func SaveDefaultUI(dir, ui string) (err error) {
	if err := validateDefaultUI(ui); err != nil {
		logger.Error("default ui invalid", "ui", ui)
		return err
	}
	dir, err = ResolveDir(dir)
	if err != nil {
		return err
	}
	lock, err := journal.LockDirectory(dir)
	if err != nil {
		logger.Error("config directory lock failed", "dir", dir, "err", err)
		return err
	}
	defer func() { err = errors.Join(err, lock.Close()) }()
	c, data, err := readConfig(dir, false)
	if err != nil {
		return err
	}
	if data == nil {
		data, err = json.Marshal(c)
	}
	if err != nil {
		logger.Error("default ui save failed", "op", "encode_default", "err", err)
		return errors.New(i18n.T(i18n.StoreConfigReadFailed))
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		logger.Error("default ui save failed", "op", "decode_config", "err", err)
		return errors.New(i18n.T(i18n.StoreConfigJsonInvalid))
	}
	fields["default_ui"], err = json.Marshal(ui)
	if err != nil {
		logger.Error("default ui save failed", "op", "encode_ui", "err", err)
		return errors.New(i18n.T(i18n.StoreConfigEncodeFailed))
	}
	data, err = json.MarshalIndent(fields, "", "  ")
	if err != nil {
		logger.Error("default ui save failed", "op", "encode_config", "err", err)
		return errors.New(i18n.T(i18n.StoreConfigEncodeFailed))
	}
	if err := writeConfig(dir, data); err != nil {
		return err
	}
	logger.Info("default ui saved", "ui", ui)
	return nil
}

func validateDefaultUI(ui string) error {
	switch ui {
	case "", "tui", "webui":
		return nil
	default:
		return errors.New(i18n.T(i18n.StoreDefaultUIInvalid))
	}
}

// readConfig 同时返回已校验的原始内容；nil 表示文件不存在。
func readConfig(dir string, secure bool) (config.Config, []byte, error) {
	c := DefaultConfig()
	path := filepath.Join(dir, "config.json")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		logger.Debug("config file missing, using defaults", "path", path)
		return c, nil, nil
	}
	if err != nil || !info.Mode().IsRegular() {
		logger.Error("config file invalid", "path", path, "err", err)
		return config.Config{}, nil, errors.New(i18n.T(i18n.StoreConfigRegularFileRequired))
	}
	if secure {
		if err := os.Chmod(path, 0600); err != nil {
			logger.Error("config file chmod failed", "path", path, "err", err)
			return config.Config{}, nil, errors.New(i18n.T(i18n.StoreConfigFileSecureFailed))
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		logger.Error("config read failed", "path", path, "err", err)
		return config.Config{}, nil, errors.New(i18n.T(i18n.StoreConfigReadFailed))
	}
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		logger.Error("config json invalid", "path", path, "err", "null document")
		return config.Config{}, nil, errors.New(i18n.T(i18n.StoreConfigJsonInvalid))
	}
	if err := json.Unmarshal(data, &c); err != nil {
		logger.Error("config json invalid", "path", path, "err", err)
		return config.Config{}, nil, errors.New(i18n.T(i18n.StoreConfigJsonInvalid))
	}
	if err := validateDefaultUI(c.DefaultUI); err != nil {
		logger.Error("config default ui invalid", "path", path, "ui", c.DefaultUI)
		return config.Config{}, nil, err
	}
	if c.Overlay, err = c.Overlay.Normalize(); err != nil {
		logger.Error("config overlay settings invalid", "path", path, "err", err)
		return config.Config{}, nil, err
	}
	if c.TTS, err = c.TTS.Normalize(); err != nil {
		logger.Error("config tts settings invalid", "path", path, "err", err)
		return config.Config{}, nil, err
	}
	if c.Blocklist, err = c.Blocklist.Normalize(); err != nil {
		logger.Error("config blocklist invalid", "path", path, "err", err)
		return config.Config{}, nil, err
	}
	applyConfigDefaults(&c)
	seen := make(map[string]bool, len(c.Accounts))
	for _, a := range c.Accounts {
		if strings.TrimSpace(a.UID) == "" || seen[a.UID] {
			logger.Error("config account index invalid", "path", path, "uid", a.UID)
			return config.Config{}, nil, errors.New(i18n.T(i18n.StoreAccountIndexInvalid))
		}
		seen[a.UID] = true
	}
	addProxySecret(c.Proxy)
	logger.Debug("config loaded", "path", path, "bytes", len(data), "accounts", len(c.Accounts), "active_uid", c.ActiveUID,
		"default_ui", c.DefaultUI, "proxy_set", c.Proxy != "")
	return c, data, nil
}

func (s *Store) Dir() string { return s.dir }
func clone(c config.Config) config.Config {
	c.Accounts = append([]domain.AccountInfo(nil), c.Accounts...)
	c.RecentTitles = append([]string(nil), c.RecentTitles...)
	c.RecentAreas = append([]domain.Area(nil), c.RecentAreas...)
	c.OverlayDisabledEvents = append([]string(nil), c.OverlayDisabledEvents...)
	if c.Overlay.Displays != nil {
		c.Overlay.Displays = append([]string{}, c.Overlay.Displays...)
	}
	c.TTS = c.TTS.Clone()
	c.Blocklist = c.Blocklist.Clone()
	return c
}
func (s *Store) Config() config.Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.overrides.Apply(s.config)
}
func (s *Store) Accounts() []domain.AccountInfo { return s.Config().Accounts }

// SaveConfig 保留由凭据存储支持的账号索引；请使用 Save/Delete 修改索引。
func (s *Store) SaveConfig(c config.Config) error {
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
	applyConfigDefaults(&c)
	var err error
	if c.TTS, err = c.TTS.Normalize(); err != nil {
		logger.Error("tts settings invalid", "op", "save_config", "err", err)
		return err
	}
	if err := s.persist(c); err != nil {
		return err
	}
	addProxySecret(c.Proxy)
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
		s.credentialFailed("load_account", uid, err)
		return domain.Account{}, secretError(i18n.T(i18n.StoreReadAccountCredentials), err)
	}
	var cookies map[string]string
	if err = json.Unmarshal([]byte(raw), &cookies); err != nil || len(cookies) == 0 {
		// 不记录解码错误，其中可能含有凭据片段。
		logger.Error("account credentials invalid", "uid", uid, "backend", storageName(s.backend.Storage()), "cookies", len(cookies))
		return domain.Account{}, errors.New(i18n.T(i18n.StoreAccountCredentialsInvalid))
	}
	addCookieSecrets(cookies)
	logger.Debug("account credentials loaded", "uid", uid, "backend", storageName(s.backend.Storage()), "cookies", len(cookies))
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
		logger.Error("account credentials encode failed", "uid", a.UID, "err", err)
		return errors.New(i18n.T(i18n.StoreAccountCredentialsEncodeFailed))
	}
	addCookieSecrets(a.Cookies)
	user := "account:" + a.UID
	old, err := s.backend.Get(user)
	exists := err == nil
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		s.credentialFailed("read_existing_account", a.UID, err)
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
	err = s.commitCredentialsLocked(c, user, old, exists, func() error {
		if err := s.backend.Set(user, string(data)); err != nil {
			s.credentialFailed("save_account", a.UID, err)
			return secretError(i18n.T(i18n.StoreSaveAccountCredentials), err)
		}
		return nil
	})
	if err == nil {
		logger.Info("account credentials saved", "uid", a.UID, "backend", storageName(s.backend.Storage()), "cookies", len(a.Cookies), "replaced", exists)
	}
	return err
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
		logger.Warn("account delete skipped, not in index", "uid", uid)
		return errors.New(i18n.T(i18n.StoreAccountNotSaved))
	}
	user := "account:" + uid
	old, err := s.backend.Get(user)
	exists := err == nil
	if err != nil && !errors.Is(err, keyring.ErrNotFound) {
		s.credentialFailed("read_before_delete", uid, err)
		return secretError(i18n.T(i18n.StoreReadCredentialsBeforeDeletion), err)
	}
	var change func() error
	if exists {
		change = func() error {
			if err := s.backend.Delete(user); err != nil {
				s.credentialFailed("delete_account", uid, err)
				return secretError(i18n.T(i18n.StoreDeleteAccountCredentials), err)
			}
			return nil
		}
	}
	c.Accounts = append(c.Accounts[:index], c.Accounts[index+1:]...)
	if c.ActiveUID == uid {
		c.ActiveUID = ""
	}
	err = s.commitCredentialsLocked(c, user, old, exists, change)
	if err == nil {
		logger.Info("account removed", "uid", uid, "had_credentials", exists)
	}
	return err
}

// commitCredentialsLocked 在凭据变更和索引持久化都成功后发布配置。
// change 为 nil 表示凭据原本不存在，不执行变更或补偿。
func (s *Store) commitCredentialsLocked(c config.Config, user, old string, exists bool, change func() error) error {
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
		logger.Error("credential rollback failed", "key", user, "backend", storageName(s.backend.Storage()), "cause", cause, "err", err)
		return fmt.Errorf(i18n.T(i18n.StoreCredentialRollbackFailed), cause)
	}
	logger.Warn("credential rolled back after index persist failure", "key", user, "backend", storageName(s.backend.Storage()), "cause", cause)
	return cause
}
func (s *Store) OBSSecret() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.backend.Get("obs-password")
	if err != nil {
		s.credentialFailed("read_obs_password", "", err)
		return "", secretError(i18n.T(i18n.StoreReadObsPassword), err)
	}
	journal.AddSecrets(v)
	return v, nil
}
func (s *Store) SetOBSSecret(password string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New(i18n.T(i18n.StoreCleared))
	}
	journal.AddSecrets(password)
	if err := s.backend.Set("obs-password", password); err != nil {
		s.credentialFailed("save_obs_password", "", err)
		return secretError(i18n.T(i18n.StoreSaveObsPassword), err)
	}
	return nil
}

// WebSocketToken 读取广播访问令牌；未设置时返回空字符串。
func (s *Store) WebSocketToken() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, err := s.backend.Get("websocket-token")
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	if err != nil {
		s.credentialFailed("read_websocket_token", "", err)
		return "", secretError(i18n.T(i18n.StoreReadWebSocketToken), err)
	}
	journal.AddSecrets(v)
	return v, nil
}

// SetWebSocketToken 保存广播访问令牌；空令牌删除已保存的值。
func (s *Store) SetWebSocketToken(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New(i18n.T(i18n.StoreCleared))
	}
	if token == "" {
		if err := s.backend.Delete("websocket-token"); err != nil && !errors.Is(err, keyring.ErrNotFound) {
			s.credentialFailed("delete_websocket_token", "", err)
			return secretError(i18n.T(i18n.StoreDeleteWebSocketToken), err)
		}
		return nil
	}
	journal.AddSecrets(token)
	if err := s.backend.Set("websocket-token", token); err != nil {
		s.credentialFailed("save_websocket_token", "", err)
		return secretError(i18n.T(i18n.StoreSaveWebSocketToken), err)
	}
	return nil
}

// 重命名是提交点：此前发生的错误不会影响原有索引。
func (s *Store) persist(c config.Config) error {
	if s.closed {
		return errors.New(i18n.T(i18n.StoreCleared))
	}
	if err := validateDefaultUI(c.DefaultUI); err != nil {
		logger.Error("config persist rejected", "reason", "default_ui", "ui", c.DefaultUI)
		return err
	}
	if _, err := c.Overlay.Normalize(); err != nil {
		logger.Error("config persist rejected", "reason", "overlay", "err", err)
		return err
	}
	if _, err := c.TTS.Normalize(); err != nil {
		logger.Error("config persist rejected", "reason", "tts", "err", err)
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		logger.Error("config encode failed", "err", err)
		return errors.New(i18n.T(i18n.StoreConfigEncodeFailed))
	}
	return writeConfig(s.dir, data)
}

func writeConfig(dir string, data []byte) error {
	f, err := os.CreateTemp(dir, ".config-*")
	if err != nil {
		logger.Error("config temp file create failed", "dir", dir, "err", err)
		return errors.New(i18n.T(i18n.StoreConfigTemporaryFileFailed))
	}
	name := f.Name()
	defer os.Remove(name)
	stage := "chmod"
	if err = f.Chmod(0600); err == nil {
		stage = "write"
		_, err = f.Write(append(data, '\n'))
	}
	if err == nil {
		stage = "sync"
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		stage = "close"
		err = closeErr
	}
	if err != nil {
		logger.Error("config write failed", "dir", dir, "stage", stage, "bytes", len(data), "err", err, "close_err", closeErr)
		return errors.New(i18n.T(i18n.StoreConfigWriteFailed))
	}
	if err = os.Rename(name, filepath.Join(dir, "config.json")); err != nil {
		logger.Error("config replace failed", "dir", dir, "err", err)
		return errors.New(i18n.T(i18n.StoreConfigReplaceFailed))
	}
	return nil
}
