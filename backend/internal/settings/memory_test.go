package settings

import (
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/provider"
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

func TestMemoryAgentReasoningEffortUsesAdvertisedLowTier(t *testing.T) {
	for _, agent := range []string{acp.AgentCodex, acp.AgentGrok} {
		if got := MemoryAgentReasoningEffort(agent); got != "low" {
			t.Fatalf("%s effort = %q, want low", agent, got)
		}
	}
	for _, agent := range []string{acp.AgentClaude, acp.AgentOpenCode} {
		if got := MemoryAgentReasoningEffort(agent); got != "" {
			t.Fatalf("%s effort = %q, want default", agent, got)
		}
	}
}

func TestMemoryAgentDefaultsCompatibleWithSupportedModels(t *testing.T) {
	cases := []struct {
		name     string
		agent    string
		defaults AgentDefaults
		model    string
		effort   string
		allowed  []string
	}{
		{
			name:    "codex",
			agent:   acp.AgentCodex,
			model:   "gpt-5.4-mini",
			effort:  "low",
			allowed: []string{"low", "medium", "high", "xhigh"},
		},
		{
			name:  "claude",
			agent: acp.AgentClaude,
			model: "sonnet",
		},
		{
			name:    "grok",
			agent:   acp.AgentGrok,
			model:   "grok-composer-2.5-fast",
			effort:  "low",
			allowed: []string{"low", "medium", "high", "xhigh"},
		},
		{
			name:     "opencode-openrouter-style",
			agent:    acp.AgentOpenCode,
			defaults: AgentDefaults{ACP: map[string]ACPAgentDefaults{acp.AgentOpenCode: {ModelProvider: provider.ProviderOpenRouter}}},
			model:    "openai/gpt-5.4-mini",
		},
		{
			name:     "opencode-openai",
			agent:    acp.AgentOpenCode,
			defaults: AgentDefaults{ACP: map[string]ACPAgentDefaults{acp.AgentOpenCode: {ModelProvider: provider.ProviderOpenAI}}},
			model:    "gpt-5.4-mini",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.defaults.ACP == nil {
				tc.defaults = DefaultAgentDefaults()
			}
			if got := MemoryAgentModel(tc.agent, tc.defaults); got != tc.model {
				t.Fatalf("model = %q, want %q", got, tc.model)
			}
			effort := MemoryAgentReasoningEffort(tc.agent)
			if effort != tc.effort {
				t.Fatalf("effort = %q, want %q", effort, tc.effort)
			}
			if effort == "" {
				return
			}
			if _, err := acp.NormalizeAgentReasoningEffort(tc.agent, effort); err != nil {
				t.Fatalf("effort %q is invalid for %s: %v", effort, tc.agent, err)
			}
			if !stringIn(tc.allowed, effort) {
				t.Fatalf("effort %q not in advertised values %v", effort, tc.allowed)
			}
		})
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

func stringIn(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
