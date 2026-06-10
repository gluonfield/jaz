package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/settingsdb"
)

func (s *Store) LoadSetting(namespace, key string) (storage.Setting, error) {
	namespace, key, err := normalizeSettingRef(namespace, key)
	if err != nil {
		return storage.Setting{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row, err := settingsdb.New(s.db).GetSetting(context.Background(), settingsdb.GetSettingParams{
		Namespace: namespace,
		Key:       key,
	})
	if err != nil {
		return storage.Setting{}, settingError(err)
	}
	return settingFromDB(row), nil
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
	s.mu.Lock()
	err = settingsdb.New(s.db).UpsertSetting(context.Background(), settingsdb.UpsertSettingParams{
		Namespace:   namespace,
		Key:         key,
		ValueJson:   string(value),
		CreatedAtMs: timeToMs(now),
		UpdatedAtMs: timeToMs(now),
	})
	s.mu.Unlock()
	if err != nil {
		return storage.Setting{}, err
	}
	return s.LoadSetting(namespace, key)
}

func (s *Store) DeleteSetting(namespace, key string) error {
	namespace, key, err := normalizeSettingRef(namespace, key)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed, err := settingsdb.New(s.db).DeleteSetting(context.Background(), settingsdb.DeleteSettingParams{
		Namespace: namespace,
		Key:       key,
	})
	if err != nil {
		return err
	}
	if changed == 0 {
		return fmt.Errorf("%w: %s/%s", storage.ErrSettingNotFound, namespace, key)
	}
	return nil
}

func (s *Store) ListSettings(namespace string) ([]storage.Setting, error) {
	namespace = strings.TrimSpace(namespace)
	if namespace == "" {
		return nil, fmt.Errorf("setting namespace is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := settingsdb.New(s.db).ListSettings(context.Background(), namespace)
	if err != nil {
		return nil, err
	}
	out := make([]storage.Setting, 0, len(rows))
	for _, row := range rows {
		out = append(out, settingFromDB(row))
	}
	return out, nil
}

func settingFromDB(row settingsdb.Setting) storage.Setting {
	return storage.Setting{
		Namespace: row.Namespace,
		Key:       row.Key,
		Value:     json.RawMessage(row.ValueJson),
		CreatedAt: msToTime(row.CreatedAtMs),
		UpdatedAt: msToTime(row.UpdatedAtMs),
	}
}

func settingError(err error) error {
	if err == sql.ErrNoRows {
		return fmt.Errorf("%w", storage.ErrSettingNotFound)
	}
	return err
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
	return namespace, key, nil
}
