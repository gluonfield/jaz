package sqlite

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func (s *Store) LoadSetting(namespace, key string) (storage.Setting, error) {
	namespace, key, err := normalizeSettingRef(namespace, key)
	if err != nil {
		return storage.Setting{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	row := s.db.QueryRow(`SELECT namespace, key, value_json, created_at_ms, updated_at_ms
FROM settings WHERE namespace = ? AND key = ?`, namespace, key)
	return scanSetting(row)
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
	_, err = s.db.Exec(`INSERT INTO settings (namespace, key, value_json, created_at_ms, updated_at_ms)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(namespace, key) DO UPDATE SET
value_json = excluded.value_json,
updated_at_ms = excluded.updated_at_ms`,
		namespace, key, string(value), timeToMs(now), timeToMs(now))
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
	res, err := s.db.Exec(`DELETE FROM settings WHERE namespace = ? AND key = ?`, namespace, key)
	if err != nil {
		return err
	}
	if changed, _ := res.RowsAffected(); changed == 0 {
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
	rows, err := s.db.Query(`SELECT namespace, key, value_json, created_at_ms, updated_at_ms
FROM settings WHERE namespace = ? ORDER BY key`, namespace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []storage.Setting
	for rows.Next() {
		setting, err := scanSetting(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, setting)
	}
	return out, rows.Err()
}

type settingScanner interface {
	Scan(dest ...any) error
}

func scanSetting(row settingScanner) (storage.Setting, error) {
	var setting storage.Setting
	var value string
	var createdMS, updatedMS int64
	if err := row.Scan(&setting.Namespace, &setting.Key, &value, &createdMS, &updatedMS); err != nil {
		if err == sql.ErrNoRows {
			return storage.Setting{}, fmt.Errorf("%w", storage.ErrSettingNotFound)
		}
		return storage.Setting{}, err
	}
	setting.Value = json.RawMessage(value)
	setting.CreatedAt = msToTime(createdMS)
	setting.UpdatedAt = msToTime(updatedMS)
	return setting, nil
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
