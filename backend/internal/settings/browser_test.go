package settings

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestBrowserSettingsDefaultDisabled(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	settings, err := LoadBrowserSettings(store)
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled {
		t.Fatalf("default browser settings = %#v", settings)
	}
}

func TestBrowserEnabledFailsClosedOnLoadError(t *testing.T) {
	if BrowserEnabled(failingSettingsStore{}) {
		t.Fatal("browser should be disabled when settings cannot be loaded")
	}
}

func TestBrowserSettingsRetainsEnablementFromLegacyRecord(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if _, err := store.SaveSetting(BrowserSettingsNamespace, BrowserSettingsKey, json.RawMessage(`{"enabled":true,"mode":"managed"}`)); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadBrowserSettings(store)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Enabled {
		t.Fatalf("settings = %#v", settings)
	}
}

type failingSettingsStore struct{}

func (failingSettingsStore) LoadSetting(string, string) (storage.Setting, error) {
	return storage.Setting{}, errors.New("settings unavailable")
}

func (failingSettingsStore) SaveSetting(string, string, json.RawMessage) (storage.Setting, error) {
	return storage.Setting{}, errors.New("settings unavailable")
}

func (failingSettingsStore) DeleteSetting(string, string) error {
	return errors.New("settings unavailable")
}

func (failingSettingsStore) ListSettings(string) ([]storage.Setting, error) {
	return nil, errors.New("settings unavailable")
}
