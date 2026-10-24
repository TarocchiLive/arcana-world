package store

import (
	"errors"
	"fmt"
	"strings"

	"arcana-world/internal/i18n"
	"arcana-world/internal/journal"
	"arcana-world/internal/tts"

	"github.com/zalando/go-keyring"
)

func ttsSecretKey(provider, field string) string {
	return "tts:" + provider + ":" + field
}

// TTSSecrets 只读取当前供应商的密钥；未配置的字段返回空值。
func (s *Store) TTSSecrets(providerID string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, errors.New(i18n.T(i18n.StoreCleared))
	}
	provider, ok := tts.ProviderByID(providerID)
	if !ok {
		return nil, fmt.Errorf(i18n.T(i18n.OutputProviderUnsupported), providerID)
	}
	values := make(map[string]string)
	for _, field := range provider.Fields {
		if !field.Secret {
			continue
		}
		value, err := s.backend.Get(ttsSecretKey(providerID, field.ID))
		if errors.Is(err, keyring.ErrNotFound) {
			continue
		}
		if err != nil {
			s.credentialFailed("read_tts_secret", "", err)
			return nil, secretError(i18n.T(i18n.TTSSecretRead), err)
		}
		journal.AddSecrets(value)
		values[field.ID] = value
	}
	return values, nil
}

// SetTTSSecret 将密钥按供应商隔离，空值表示删除。
func (s *Store) SetTTSSecret(providerID, fieldID, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return errors.New(i18n.T(i18n.StoreCleared))
	}
	provider, ok := tts.ProviderByID(providerID)
	if !ok {
		return fmt.Errorf(i18n.T(i18n.OutputProviderUnsupported), providerID)
	}
	known := false
	for _, field := range provider.Fields {
		if field.ID == fieldID && field.Secret {
			known = true
			break
		}
	}
	if !known {
		return errors.New("tts: unknown secret field")
	}
	value = strings.TrimSpace(value)
	journal.AddSecrets(value)
	settings := tts.DefaultSettings()
	settings.Provider = providerID
	if _, err := settings.WithValue(fieldID, value).Normalize(); err != nil {
		return err
	}
	key := ttsSecretKey(providerID, fieldID)
	var err error
	if value == "" {
		err = s.backend.Delete(key)
		if errors.Is(err, keyring.ErrNotFound) {
			err = nil
		}
	} else {
		err = s.backend.Set(key, value)
	}
	if err != nil {
		s.credentialFailed("save_tts_secret", "", err)
		return secretError(i18n.T(i18n.TTSSecretSave), err)
	}
	return nil
}
