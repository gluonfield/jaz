package settings

import (
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestDefaultWorkerAgentPriority(t *testing.T) {
	defaults := AgentDefaults{ACP: map[string]ACPAgentDefaults{
		acp.AgentClaude:   {Enabled: true},
		acp.AgentOpenCode: {Enabled: true},
	}}
	if got := DefaultWorkerAgent(defaults); got != acp.AgentClaude {
		t.Fatalf("agent = %q", got)
	}

	defaults.ACP[acp.AgentCodex] = ACPAgentDefaults{Enabled: true}
	if got := DefaultWorkerAgent(defaults); got != acp.AgentCodex {
		t.Fatalf("agent = %q", got)
	}
}

func TestWorkerAgentReasoningEffortUsesXHighWhenAvailable(t *testing.T) {
	defaults := AgentDefaults{ACP: map[string]ACPAgentDefaults{
		acp.AgentOpenCode: {ModelProvider: provider.ProviderOpenRouter},
	}}
	for _, agent := range []string{acp.AgentCodex, acp.AgentClaude, acp.AgentGrok, acp.AgentOpenCode} {
		if got := WorkerAgentReasoningEffort(agent, defaults); got != "xhigh" {
			t.Fatalf("%s effort = %q, want xhigh", agent, got)
		}
	}
	for _, agent := range []string{acp.AgentAntigravity} {
		if got := WorkerAgentReasoningEffort(agent, defaults); got != "" {
			t.Fatalf("%s effort = %q, want default", agent, got)
		}
	}
	for _, agent := range []string{"", "custom"} {
		if got := WorkerAgentReasoningEffort(agent, defaults); got != "" {
			t.Fatalf("%q effort = %q, want default", agent, got)
		}
	}
	defaults.ACP[acp.AgentOpenCode] = ACPAgentDefaults{ModelProvider: provider.ProviderOllama}
	if got := WorkerAgentReasoningEffort(acp.AgentOpenCode, defaults); got != "" {
		t.Fatalf("opencode/ollama effort = %q, want no default", got)
	}
}

func TestWorkerAgentDefaultsCompatibleWithSupportedModels(t *testing.T) {
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
			effort:  "xhigh",
			allowed: []string{"low", "medium", "high", "xhigh"},
		},
		{
			name:    "claude",
			agent:   acp.AgentClaude,
			model:   "default",
			effort:  "xhigh",
			allowed: []string{"low", "medium", "high", "xhigh", "max", "ultracode"},
		},
		{
			name:    "grok",
			agent:   acp.AgentGrok,
			model:   modelcatalog.DefaultGrokModel,
			effort:  "xhigh",
			allowed: []string{"low", "medium", "high", "xhigh"},
		},
		{
			name:     "opencode-openrouter-style",
			agent:    acp.AgentOpenCode,
			defaults: AgentDefaults{ACP: map[string]ACPAgentDefaults{acp.AgentOpenCode: {ModelProvider: provider.ProviderOpenRouter}}},
			model:    "z-ai/glm-5.2",
			effort:   "xhigh",
			allowed:  []string{"low", "medium", "high", "xhigh", "max"},
		},
		{
			name:     "opencode-openai",
			agent:    acp.AgentOpenCode,
			defaults: AgentDefaults{ACP: map[string]ACPAgentDefaults{acp.AgentOpenCode: {ModelProvider: provider.ProviderOpenAI}}},
			model:    "gpt-5.4-mini",
			effort:   "xhigh",
			allowed:  []string{"low", "medium", "high", "xhigh", "max"},
		},
		{
			name:     "opencode-ollama",
			agent:    acp.AgentOpenCode,
			defaults: AgentDefaults{ACP: map[string]ACPAgentDefaults{acp.AgentOpenCode: {ModelProvider: provider.ProviderOllama}}},
		},
		{
			name:     "opencode-custom-provider",
			agent:    acp.AgentOpenCode,
			defaults: AgentDefaults{ACP: map[string]ACPAgentDefaults{acp.AgentOpenCode: {ModelProvider: "internal"}}},
		},
		{
			name:  "antigravity",
			agent: acp.AgentAntigravity,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.defaults.ACP == nil {
				tc.defaults = DefaultAgentDefaults()
			}
			if got := WorkerAgentModel(tc.agent, tc.defaults); got != tc.model {
				t.Fatalf("model = %q, want %q", got, tc.model)
			}
			effort := WorkerAgentReasoningEffort(tc.agent, tc.defaults)
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

func TestMemorySettingsWorkerOverrides(t *testing.T) {
	defaults := DefaultAgentDefaults()
	settings := MemorySettings{Agent: acp.AgentClaude}
	if got := settings.WorkerModel(defaults); got != "default" {
		t.Fatalf("default model = %q, want default", got)
	}
	if got := settings.WorkerReasoningEffort(defaults); got != "xhigh" {
		t.Fatalf("default effort = %q, want xhigh", got)
	}
	settings.Model = "haiku"
	settings.ReasoningEffort = "low"
	if got := settings.WorkerModel(defaults); got != "haiku" {
		t.Fatalf("override model = %q, want haiku", got)
	}
	if got := settings.WorkerReasoningEffort(defaults); got != "low" {
		t.Fatalf("override effort = %q, want low", got)
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
