package storage

import (
	"encoding/json"
	"errors"
	"time"
)

var ErrSettingNotFound = errors.New("setting not found")

type Setting struct {
	Namespace string          `json:"namespace"`
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type SettingsStorage interface {
	LoadSetting(namespace, key string) (Setting, error)
	SaveSetting(namespace, key string, value json.RawMessage) (Setting, error)
	DeleteSetting(namespace, key string) error
	ListSettings(namespace string) ([]Setting, error)
}
