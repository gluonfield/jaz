package settings

import (
	"testing"

	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestDeviceSettingsPersistNormalizedPublicURL(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	saved, err := SaveDeviceSettings(store, DeviceSettings{PublicURL: " jaz.example.com/ "})
	if err != nil {
		t.Fatal(err)
	}
	if saved.PublicURL != "https://jaz.example.com" {
		t.Fatalf("saved public url = %q", saved.PublicURL)
	}
	loaded, err := LoadDeviceSettings(store)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != saved {
		t.Fatalf("loaded settings = %#v, want %#v", loaded, saved)
	}
}

func TestDeviceSettingsRejectPublicURLWithPath(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := SaveDeviceSettings(store, DeviceSettings{PublicURL: "https://jaz.example.com/app"}); err == nil {
		t.Fatal("expected public URL path to be rejected")
	}
}
