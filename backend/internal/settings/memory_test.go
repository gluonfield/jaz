package settings

import (
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestDefaultMemoryAgentPriority(t *testing.T) {
	defaults := AgentDefaults{ACP: map[string]ACPAgentDefaults{
		acp.AgentClaude:   {Enabled: true},
		acp.AgentOpenCode: {Enabled: true},
	}}
	if got := DefaultMemoryAgent(defaults); got != acp.AgentClaude {
		t.Fatalf("agent = %q", got)
	}

	defaults.ACP[acp.AgentCodex] = ACPAgentDefaults{Enabled: true}
	if got := DefaultMemoryAgent(defaults); got != acp.AgentCodex {
		t.Fatalf("agent = %q", got)
	}
}

func TestLoadMemorySettingsReadsLegacyAgentFields(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.SaveSetting(MemorySettingsNamespace, MemorySettingsKey, []byte(`{
		"enabled": true,
		"dream_agent": "claude",
		"search_agent": "codex"
	}`)); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadMemorySettings(store)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Enabled || settings.Agent != acp.AgentCodex {
		t.Fatalf("settings = %#v", settings)
	}
}
