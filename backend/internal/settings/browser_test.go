package settings

import (
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
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
