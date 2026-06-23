package settings

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

const (
	BrowserSettingsNamespace = "browser"
	BrowserSettingsKey       = "settings"
)

type BrowserSettings struct {
	Enabled bool   `json:"enabled"`
	Agent   string `json:"agent,omitempty"`
}

func DefaultBrowserSettings() BrowserSettings {
	return BrowserSettings{Enabled: true}
}

func LoadBrowserSettings(store storage.SettingsStorage) (BrowserSettings, error) {
	setting, err := store.LoadSetting(BrowserSettingsNamespace, BrowserSettingsKey)
	if errors.Is(err, storage.ErrSettingNotFound) {
		return DefaultBrowserSettings(), nil
	}
	if err != nil {
		return BrowserSettings{}, err
	}
	var settings BrowserSettings
	if err := json.Unmarshal(setting.Value, &settings); err != nil {
		return BrowserSettings{}, err
	}
	settings.Agent = strings.TrimSpace(settings.Agent)
	return settings, nil
}

func SaveBrowserSettings(store storage.SettingsStorage, settings BrowserSettings) (BrowserSettings, error) {
	settings.Agent = strings.TrimSpace(settings.Agent)
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

func BrowserAgent(settings BrowserSettings, defaults AgentDefaults) string {
	if agent := acp.CanonicalAgentName(settings.Agent); agent != "" {
		return agent
	}
	return DefaultWorkerAgent(defaults)
}
