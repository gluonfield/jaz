package settings

import (
	"encoding/json"
	"errors"
	"net/url"
	"strings"

	"github.com/wins/jaz/backend/internal/storage"
)

var ErrInvalidDevicePublicURL = errors.New("connection address must be an HTTP or HTTPS origin")

const (
	DeviceSettingsNamespace = "devices"
	DeviceSettingsKey       = "settings"
)

type DeviceSettings struct {
	PublicURL string `json:"public_url,omitempty"`
}

func LoadDeviceSettings(store storage.SettingsStorage) (DeviceSettings, error) {
	setting, err := store.LoadSetting(DeviceSettingsNamespace, DeviceSettingsKey)
	if errors.Is(err, storage.ErrSettingNotFound) {
		return DeviceSettings{}, nil
	}
	if err != nil {
		return DeviceSettings{}, err
	}
	var settings DeviceSettings
	if err := json.Unmarshal(setting.Value, &settings); err != nil {
		return DeviceSettings{}, err
	}
	settings.PublicURL, err = normalizePublicURL(settings.PublicURL)
	return settings, err
}

func SaveDeviceSettings(store storage.SettingsStorage, settings DeviceSettings) (DeviceSettings, error) {
	publicURL, err := normalizePublicURL(settings.PublicURL)
	if err != nil {
		return DeviceSettings{}, err
	}
	settings.PublicURL = publicURL
	data, err := json.Marshal(settings)
	if err != nil {
		return DeviceSettings{}, err
	}
	if _, err := store.SaveSetting(DeviceSettingsNamespace, DeviceSettingsKey, data); err != nil {
		return DeviceSettings{}, err
	}
	return settings, nil
}

func normalizePublicURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
		return "", ErrInvalidDevicePublicURL
	}
	u.Path = ""
	return u.String(), nil
}
