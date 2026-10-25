package settings

import (
	"encoding/json"
	"errors"

	"github.com/wins/jaz/backend/internal/storage"
)

const (
	BrowserSettingsNamespace = "browser"
	BrowserSettingsKey       = "settings"
)

type BrowserSettings struct {
	Enabled bool `json:"enabled"`
}

func LoadBrowserSettings(store storage.SettingsStorage) (BrowserSettings, error) {
	setting, err := store.LoadSetting(BrowserSettingsNamespace, BrowserSettingsKey)
	if errors.Is(err, storage.ErrSettingNotFound) {
		return BrowserSettings{}, nil
	}
	if err != nil {
		return BrowserSettings{}, err
	}
	var settings BrowserSettings
	if err := json.Unmarshal(setting.Value, &settings); err != nil {
		return BrowserSettings{}, err
	}
	return settings, nil
}

func SaveBrowserSettings(store storage.SettingsStorage, settings BrowserSettings) (BrowserSettings, error) {
	data, err := json.Marshal(settings)
	if err != nil {
		return BrowserSettings{}, err
	}
	if _, err := store.SaveSetting(BrowserSettingsNamespace, BrowserSettingsKey, data); err != nil {
		return BrowserSettings{}, err
	}
	return settings, nil
}

func BrowserEnabled(store storage.SettingsStorage) bool {
	settings, err := LoadBrowserSettings(store)
	if err != nil {
		return false
	}
	return settings.Enabled
}
