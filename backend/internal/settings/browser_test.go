package settings

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestBrowserSettingsDefaultEnabled(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	settings, err := LoadBrowserSettings(store)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Enabled {
		t.Fatalf("default browser settings = %#v", settings)
	}
}

func TestBrowserEnabledFailsClosedOnLoadError(t *testing.T) {
	if BrowserEnabled(failingSettingsStore{}) {
		t.Fatal("browser should be disabled when settings cannot be loaded")
	}
}

func TestBrowserAgentDefaultsToEnabledWorkerAgent(t *testing.T) {
	defaults := AgentDefaults{ACP: map[string]ACPAgentDefaults{
		acp.AgentClaude: {Enabled: true},
	}}
	if got := BrowserAgent(BrowserSettings{Enabled: true}, defaults); got != acp.AgentClaude {
		t.Fatalf("agent = %q", got)
	}
	if got := BrowserAgent(BrowserSettings{Enabled: true, Agent: acp.AgentCodex}, defaults); got != acp.AgentCodex {
		t.Fatalf("explicit agent = %q", got)
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
