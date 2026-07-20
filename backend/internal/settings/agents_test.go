package settings

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/modelcatalog"
	"github.com/wins/jaz/backend/internal/provider"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func testAgentDefaultsSeed() AgentDefaults {
	return AgentDefaultsFromCatalog(acp.BuiltinAgents())
}

func TestAgentDefaultsFromCatalogKeepsAuthManagedAgentsDisabled(t *testing.T) {
	seed := AgentDefaultsFromCatalog(acp.BuiltinAgents())
	for _, agent := range []string{acp.AgentCodex, acp.AgentClaude, acp.AgentKimi, acp.AgentGrok, acp.AgentOpenCode, acp.AgentAntigravity} {
		if seed.ACP[agent].Enabled {
			t.Fatalf("%s seeded enabled without auth: %#v", agent, seed.ACP[agent])
		}
	}
	custom := AgentDefaultsFromCatalog(acp.AgentCatalog{
		"local_helper":  {Command: "/opt/jaz/local-helper"},
		"remote_helper": {URL: "http://127.0.0.1:7777/acp"},
	})
	if !custom.ACP["local_helper"].Enabled || !custom.ACP["remote_helper"].Enabled {
		t.Fatalf("custom agents should seed enabled: %#v", custom.ACP)
	}
}

// The launch command is taken straight from the catalog: a command-required
// agent (grok) gets the catalog command, and a managed agent (codex) gets its
// bundled adapter. User settings never carry a command.
func TestACPConfigSourceUsesCatalogLaunchCommand(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stored := testAgentDefaultsSeed()
	for name, agent := range stored.ACP {
		agent.Enabled = true
		stored.ACP[name] = agent
	}
	if _, err := SaveAgentDefaults(store, stored); err != nil {
		t.Fatal(err)
	}
	source := NewACPConfigSource(store, acp.BuiltinAgents())

	grok, ok, err := source.AgentConfig("grok")
	if err != nil || !ok {
		t.Fatalf("grok config: ok=%v err=%v", ok, err)
	}
	if grok.Command != "grok" {
		t.Fatalf("grok command = %q, want catalog default", grok.Command)
	}

	codex, ok, err := source.AgentConfig("codex")
	if err != nil || !ok {
		t.Fatalf("codex config: ok=%v err=%v", ok, err)
	}
	if codex.Command != "" || codex.ManagedAdapter != "codex" {
		t.Fatalf("codex config = %#v, want managed adapter", codex)
	}
}

func TestNormalizeAgentDefaultsAllowsClaudeOnlyReasoningEffort(t *testing.T) {
	for _, effort := range []string{"max", "ultracode"} {
		input := testAgentDefaultsSeed()
		claude := input.ACP["claude"]
		claude.ReasoningEffort = effort
		input.ACP["claude"] = claude

		normalized, err := NormalizeAgentDefaults(input, acp.BuiltinAgents())
		if err != nil {
			t.Fatalf("NormalizeAgentDefaults(%q) error: %v", effort, err)
		}
		if normalized.ACP["claude"].ReasoningEffort != effort {
			t.Fatalf("claude effort = %q, want %q", normalized.ACP["claude"].ReasoningEffort, effort)
		}
	}
}

func TestNormalizeAgentDefaultsRejectsClaudeOnlyReasoningForOtherACPAgents(t *testing.T) {
	input := testAgentDefaultsSeed()
	codex := input.ACP["codex"]
	codex.ReasoningEffort = "ultracode"
	input.ACP["codex"] = codex

	if _, err := NormalizeAgentDefaults(input, acp.BuiltinAgents()); err == nil {
		t.Fatal("expected codex ultracode effort to be rejected")
	}
}

func TestNormalizeAgentDefaultsAllowsCodexUltra(t *testing.T) {
	builtin := acp.BuiltinAgents()
	catalog := acp.AgentCatalog{acp.AgentCodex: builtin[acp.AgentCodex]}
	input := AgentDefaultsFromCatalog(catalog)
	codex := input.ACP[acp.AgentCodex]
	codex.Model = acp.CodexOpenAIDefaultModel
	codex.ReasoningEffort = "ultra"
	input.ACP[acp.AgentCodex] = codex

	service := warmSettingsModelCatalog(t, `{"data":[{"id":"openai/gpt-5.6-sol","reasoning":{"supported_efforts":["xhigh","high","medium","low"]}}]}`)
	normalized, err := NormalizeAgentDefaults(input, catalog, acp.ModelCapabilities{Catalog: service})
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ACP[acp.AgentCodex].ReasoningEffort != "ultra" {
		t.Fatalf("codex effort = %q, want ultra", normalized.ACP[acp.AgentCodex].ReasoningEffort)
	}
}

func TestNormalizeAgentDefaultsRejectsModelSpecificUnsupportedReasoning(t *testing.T) {
	input := testAgentDefaultsSeed()
	claude := input.ACP["claude"]
	claude.Model = "sonnet"
	claude.ReasoningEffort = "minimal"
	input.ACP["claude"] = claude

	service := warmSettingsModelCatalog(t, `{"data":[{"id":"anthropic/claude-sonnet-5","reasoning":{"supported_efforts":["max","high","medium","low"]}}]}`)

	_, err := NormalizeAgentDefaults(input, acp.BuiltinAgents(), acp.ModelCapabilities{Catalog: service})
	if err == nil || !strings.Contains(err.Error(), `reasoning effort "minimal" is not supported for claude model "sonnet"`) {
		t.Fatalf("err = %v", err)
	}
}

func warmSettingsModelCatalog(t *testing.T, body string) *modelcatalog.Service {
	t.Helper()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(upstream.Close)
	service := modelcatalog.NewService(provider.StaticSource(map[string]provider.ModelProviderConfig{
		provider.ProviderOpenRouter: {BaseURL: upstream.URL},
	}))
	if err := service.Warm(context.Background()); err != nil {
		t.Fatal(err)
	}
	return service
}

func TestNormalizeAgentDefaultsSplitsOpenCodeProviderModel(t *testing.T) {
	input := testAgentDefaultsSeed()
	opencode := input.ACP["opencode"]
	opencode.ModelProvider = ""
	opencode.Model = "openrouter/openai/gpt-5.5"
	input.ACP["opencode"] = opencode

	normalized, err := NormalizeAgentDefaults(input, acp.BuiltinAgents())
	if err != nil {
		t.Fatal(err)
	}
	got := normalized.ACP["opencode"]
	if got.ModelProvider != "openrouter" || got.Model != "openai/gpt-5.5" {
		t.Fatalf("opencode defaults = %#v", got)
	}
}

func TestNormalizeAgentDefaultsPreservesACPAuthProfile(t *testing.T) {
	input := testAgentDefaultsSeed()
	codex := input.ACP["codex"]
	codex.Auth = acp.AgentAuthConfig{Mode: acp.AuthModeExistingCLI, Path: "~/custom-codex"}
	input.ACP["codex"] = codex

	normalized, err := NormalizeAgentDefaults(input, acp.BuiltinAgents())
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ACP["codex"].Auth.Mode != acp.AuthModeExistingCLI {
		t.Fatalf("auth = %#v", normalized.ACP["codex"].Auth)
	}
	if normalized.ACP["codex"].Auth.Path != "~/custom-codex" {
		t.Fatalf("auth path = %q", normalized.ACP["codex"].Auth.Path)
	}
}

func TestNormalizeAgentDefaultsRejectsGrokPathBearingAuth(t *testing.T) {
	for _, auth := range []acp.AgentAuthConfig{
		{Mode: acp.AuthModeJazProfile},
		{Mode: acp.AuthModeExistingCLI, Path: "~/custom-grok"},
	} {
		input := testAgentDefaultsSeed()
		grok := input.ACP["grok"]
		grok.Auth = auth
		input.ACP["grok"] = grok

		if _, err := NormalizeAgentDefaults(input, acp.BuiltinAgents()); err == nil {
			t.Fatalf("expected grok auth %#v to be rejected", auth)
		}
	}
}

func TestMergeAgentDefaultsDropsInvalidGrokAuthProfile(t *testing.T) {
	seed := testAgentDefaultsSeed()
	stored := AgentDefaults{
		ACP: map[string]ACPAgentDefaults{},
	}
	for name, agent := range seed.ACP {
		stored.ACP[name] = agent
	}
	grok := stored.ACP["grok"]
	grok.Auth = acp.AgentAuthConfig{Mode: acp.AuthModeJazProfile, Path: "~/fake-grok"}
	stored.ACP["grok"] = grok

	merged := MergeAgentDefaults(stored, seed, agentNames(seed))

	if merged.ACP["grok"].Auth.Mode != acp.AuthModeAuto || merged.ACP["grok"].Auth.Path != "" {
		t.Fatalf("grok auth = %#v, want auto without path", merged.ACP["grok"].Auth)
	}
}

func TestMergeAgentDefaultsMigratesRetiredGrokBuildModel(t *testing.T) {
	seed := testAgentDefaultsSeed()
	stored := AgentDefaults{ACP: map[string]ACPAgentDefaults{}}
	for name, agent := range seed.ACP {
		stored.ACP[name] = agent
	}
	grok := stored.ACP[acp.AgentGrok]
	grok.Model = "grok-build"
	stored.ACP[acp.AgentGrok] = grok

	merged := MergeAgentDefaults(stored, seed, agentNames(seed))

	if merged.ACP[acp.AgentGrok].Model != seed.ACP[acp.AgentGrok].Model {
		t.Fatalf("grok model = %q, want %q", merged.ACP[acp.AgentGrok].Model, seed.ACP[acp.AgentGrok].Model)
	}
}

func TestACPConfigSourceClearsRetiredGrokReasoningEffort(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seed := testAgentDefaultsSeed()
	stored := testAgentDefaultsSeed()
	grok := stored.ACP[acp.AgentGrok]
	grok.Enabled = true
	grok.Model = "grok-composer-2.5-fast"
	grok.ReasoningEffort = "xhigh"
	stored.ACP[acp.AgentGrok] = grok
	if _, err := SaveAgentDefaults(store, stored); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAgentDefaults(store, seed); err != nil {
		t.Fatal(err)
	}

	cfg, ok, err := NewACPConfigSource(store, acp.BuiltinAgents(), acp.ModelCapabilities{Catalog: modelcatalog.NewService(nil)}).AgentConfig(acp.AgentGrok)
	if err != nil || !ok {
		t.Fatalf("grok config: ok=%v err=%v", ok, err)
	}
	if cfg.Model != grok.Model || cfg.ReasoningEffort != "" {
		t.Fatalf("grok config = %#v, want Composer without an effort", cfg)
	}
}
