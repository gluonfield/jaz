package jsonstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Store) LoadSetting(namespace, key string) (storage.Setting, error) {
	namespace, key, err := normalizeSettingRef(namespace, key)
	if err != nil {
		return storage.Setting{}, err
	}
	data, err := os.ReadFile(s.settingPath(namespace, key))
	if err != nil {
		if os.IsNotExist(err) {
			return storage.Setting{}, fmt.Errorf("%w: %s/%s", storage.ErrSettingNotFound, namespace, key)
		}
		return storage.Setting{}, err
	}
	var setting storage.Setting
	if err := json.Unmarshal(data, &setting); err != nil {
		return storage.Setting{}, err
	}
	if len(setting.Value) == 0 || !json.Valid(setting.Value) {
		return storage.Setting{}, fmt.Errorf("setting %s/%s value must be valid JSON", namespace, key)
	}
	return setting, nil
}

func (s *Store) SaveSetting(namespace, key string, value json.RawMessage) (storage.Setting, error) {
	namespace, key, err := normalizeSettingRef(namespace, key)
	if err != nil {
		return storage.Setting{}, err
	}
	value = append(json.RawMessage(nil), value...)
	if len(value) == 0 || !json.Valid(value) {
		return storage.Setting{}, fmt.Errorf("setting %s/%s value must be valid JSON", namespace, key)
	}
	now := time.Now().UTC()
	created := now
	if existing, err := s.LoadSetting(namespace, key); err == nil && !existing.CreatedAt.IsZero() {
		created = existing.CreatedAt
	}
	setting := storage.Setting{
		Namespace: namespace,
		Key:       key,
		Value:     value,
		CreatedAt: created,
		UpdatedAt: now,
	}
	if err := os.MkdirAll(filepath.Dir(s.settingPath(namespace, key)), 0o755); err != nil {
		return storage.Setting{}, err
	}
	data, err := json.MarshalIndent(setting, "", "  ")
	if err != nil {
		return storage.Setting{}, err
	}
	if err := os.WriteFile(s.settingPath(namespace, key), data, 0o644); err != nil {
		return storage.Setting{}, err
	}
	return setting, nil
}

func (s *Store) DeleteSetting(namespace, key string) error {
	namespace, key, err := normalizeSettingRef(namespace, key)
	if err != nil {
		return err
	}
	if err := os.Remove(s.settingPath(namespace, key)); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%w: %s/%s", storage.ErrSettingNotFound, namespace, key)
		}
		return err
	}
	return nil
}

func (s *Store) ListSettings(namespace string) ([]storage.Setting, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, fmt.Errorf("setting namespace is required")
	}
	entries, err := os.ReadDir(s.settingNamespaceDir(namespace))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []storage.Setting
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		key := strings.TrimSuffix(entry.Name(), ".json")
		setting, err := s.LoadSetting(namespace, key)
		if err != nil {
			return nil, err
		}
		out = append(out, setting)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func (s *Store) settingNamespaceDir(namespace string) string {
	return filepath.Join(s.root, "settings", namespace)
}

func (s *Store) settingPath(namespace, key string) string {
	return filepath.Join(s.settingNamespaceDir(namespace), key+".json")
}

func normalizeSettingRef(namespace, key string) (string, string, error) {
	namespace = strings.TrimSpace(namespace)
	key = strings.TrimSpace(key)
	if namespace == "" {
		return "", "", fmt.Errorf("setting namespace is required")
	}
	if key == "" {
		return "", "", fmt.Errorf("setting key is required")
	}
	if strings.Contains(namespace, "/") || strings.Contains(key, "/") {
		return "", "", fmt.Errorf("setting namespace and key must not contain slashes")
	}
	return namespace, key, nil
}
