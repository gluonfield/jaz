package settings

import (
	"encoding/json"
	"errors"

	"github.com/wins/jaz/backend/internal/storage"
)

const (
	MemorySettingsNamespace = "memory"
	MemorySettingsKey       = "settings"
)

type MemorySettings struct {
	Enabled bool `json:"enabled"`
}

func DefaultMemorySettings() MemorySettings {
	return MemorySettings{Enabled: true}
}

func LoadMemorySettings(store storage.SettingsStorage) (MemorySettings, error) {
	setting, err := store.LoadSetting(MemorySettingsNamespace, MemorySettingsKey)
	if errors.Is(err, storage.ErrSettingNotFound) {
		return DefaultMemorySettings(), nil
	}
	if err != nil {
		return MemorySettings{}, err
	}
	var out MemorySettings
	if err := json.Unmarshal(setting.Value, &out); err != nil {
		return MemorySettings{}, err
	}
	return out, nil
}

func SaveMemorySettings(store storage.SettingsStorage, settings MemorySettings) (MemorySettings, error) {
	data, err := json.Marshal(settings)
	if err != nil {
		return MemorySettings{}, err
	}
	if _, err := store.SaveSetting(MemorySettingsNamespace, MemorySettingsKey, data); err != nil {
		return MemorySettings{}, err
	}
	return settings, nil
}

// MemoryEnabled is the live gate used per turn, spawn, and request; storage
// errors fail open so a settings hiccup never silently disables memory.
func MemoryEnabled(store storage.SettingsStorage) bool {
	settings, err := LoadMemorySettings(store)
	if err != nil {
		return true
	}
	return settings.Enabled
}
