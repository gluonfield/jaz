package settings

import (
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func testAgentDefaultsSeed() AgentDefaults {
	return AgentDefaultsFromCatalog(acp.BuiltinAgents())
}

func TestAgentDefaultsFromCatalogKeepsAuthManagedAgentsDisabled(t *testing.T) {
	seed := AgentDefaultsFromCatalog(acp.BuiltinAgents())
	for _, agent := range []string{acp.AgentCodex, acp.AgentClaude, acp.AgentGrok, acp.AgentOpenCode} {
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

func TestParseCommandLinePreservesQuotedArgs(t *testing.T) {
	command, args, err := ParseCommandLine(`/opt/jaz/codex-acp -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'`)
	if err != nil {
		t.Fatal(err)
	}

	if command != "/opt/jaz/codex-acp" {
		t.Fatalf("command = %q", command)
	}
	want := []string{
		"-c",
		`sandbox_mode="danger-full-access"`,
		"-c",
		`approval_policy="never"`,
	}
	if strings.Join(args, "\n") != strings.Join(want, "\n") {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
}

func TestParseCommandLineRejectsUnterminatedQuote(t *testing.T) {
	_, _, err := ParseCommandLine(`/opt/jaz/codex-acp -c 'sandbox_mode="danger-full-access"`)
	if err == nil || !strings.Contains(err.Error(), "unterminated") {
		t.Fatalf("err = %v, want unterminated quote", err)
	}
}

func TestEnsureAgentDefaultsDropsManagedAgentCommand(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seed := testAgentDefaultsSeed()
	custom := seed
	custom.ACP = map[string]ACPAgentDefaults{
		"codex": {
			Enabled:         true,
			Command:         "/custom/codex-acp",
			Model:           "gpt-5.5",
			ReasoningEffort: "medium",
		},
	}
	if _, err := SaveAgentDefaults(store, custom); err != nil {
		t.Fatal(err)
	}

	if err := EnsureAgentDefaults(store, seed); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAgentDefaults(store)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ACP["codex"].Command != "" {
		t.Fatalf("codex command = %q, want managed default", loaded.ACP["codex"].Command)
	}
}

func TestEnsureAgentDefaultsRefreshesLegacyCodexBuiltinCommand(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seed := testAgentDefaultsSeed()
	stored := seed
	stored.ACP = map[string]ACPAgentDefaults{}
	for name, agent := range seed.ACP {
		stored.ACP[name] = agent
	}
	codex := stored.ACP["codex"]
	codex.Command = legacyCodexCommand("@jazchat/codex-acp@0.16.1")
	stored.ACP["codex"] = codex
	if _, err := SaveAgentDefaults(store, stored); err != nil {
		t.Fatal(err)
	}

	if err := EnsureAgentDefaults(store, seed); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAgentDefaults(store)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ACP["codex"].Command != "" {
		t.Fatalf("codex command = %q, want managed default", loaded.ACP["codex"].Command)
	}
}

func TestMergeAgentDefaultsDropsManagedAgentCommand(t *testing.T) {
	seed := testAgentDefaultsSeed()
	stored := AgentDefaults{ACP: map[string]ACPAgentDefaults{
		"codex":  {Command: `/custom/codex-acp --stdio`},
		"claude": {Command: legacyClaudeCommand("@agentclientprotocol/claude-agent-acp@0.51.0")},
	}}

	merged := MergeAgentDefaults(stored, seed, []string{"codex", "claude"})

	if merged.ACP["codex"].Command != "" || merged.ACP["claude"].Command != "" {
		t.Fatalf("managed commands = codex %q claude %q, want empty", merged.ACP["codex"].Command, merged.ACP["claude"].Command)
	}
}

func TestACPConfigSourceUsesMergedCodexBuiltinCommand(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seed := testAgentDefaultsSeed()
	stored := seed
	stored.ACP = map[string]ACPAgentDefaults{}
	for name, agent := range seed.ACP {
		stored.ACP[name] = agent
	}
	codex := stored.ACP["codex"]
	codex.Enabled = true
	codex.Command = legacyCodexCommand("@jazchat/codex-acp@0.16.6")
	stored.ACP["codex"] = codex
	if _, err := SaveAgentDefaults(store, stored); err != nil {
		t.Fatal(err)
	}

	cfg, ok, err := NewACPConfigSource(store, acp.BuiltinAgents()).AgentConfig("codex")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("codex config disabled")
	}
	if cfg.Command != "" || cfg.ManagedAdapter != "codex" {
		t.Fatalf("codex config = %#v, want managed adapter", cfg)
	}
	if !strings.Contains(strings.Join(cfg.ManagedAdapterArgs, "\n"), `sandbox_mode="danger-full-access"`) {
		t.Fatalf("codex managed args = %#v", cfg.ManagedAdapterArgs)
	}
}

func TestMergeAgentDefaultsRefreshesLegacyClaudeBuiltinCommand(t *testing.T) {
	seed := testAgentDefaultsSeed()
	stored := AgentDefaults{ACP: map[string]ACPAgentDefaults{
		"claude": {Command: legacyClaudeCommand("@agentclientprotocol/claude-agent-acp@0.44.0")},
	}}

	merged := MergeAgentDefaults(stored, seed, []string{"claude"})

	if merged.ACP["claude"].Command != "" {
		t.Fatalf("claude command = %q, want managed default", merged.ACP["claude"].Command)
	}
}

func TestACPConfigSourceUsesMergedClaudeBuiltinCommand(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seed := testAgentDefaultsSeed()
	stored := seed
	stored.ACP = map[string]ACPAgentDefaults{}
	for name, agent := range seed.ACP {
		stored.ACP[name] = agent
	}
	claude := stored.ACP["claude"]
	claude.Enabled = true
	claude.Command = legacyClaudeCommand("@agentclientprotocol/claude-agent-acp@0.44.0")
	stored.ACP["claude"] = claude
	if _, err := SaveAgentDefaults(store, stored); err != nil {
		t.Fatal(err)
	}

	cfg, ok, err := NewACPConfigSource(store, acp.BuiltinAgents()).AgentConfig("claude")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("claude config disabled")
	}
	if cfg.Command != "" || cfg.ManagedAdapter != "claude" {
		t.Fatalf("claude config = %#v, want managed adapter", cfg)
	}
}

func TestMergeAgentDefaultsRefreshesLegacyCodexCommandBeforeToolSearchFlag(t *testing.T) {
	seed := testAgentDefaultsSeed()
	stored := AgentDefaults{ACP: map[string]ACPAgentDefaults{
		"codex": {
			Command:         `npx -y @jazchat/codex-acp@0.16.1 -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"'`,
			Model:           "gpt-5.5",
			ReasoningEffort: "xhigh",
		},
	}}

	merged := MergeAgentDefaults(stored, seed, []string{"codex"})

	if merged.ACP["codex"].Command != "" {
		t.Fatalf("codex command = %q, want managed default", merged.ACP["codex"].Command)
	}
}

func legacyCodexCommand(pkg string) string {
	return `npx -y ` + pkg + ` -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"' -c features.tool_search_always_defer_mcp_tools=true -c suppress_unstable_features_warning=true`
}

func legacyClaudeCommand(pkg string) string {
	return `npx -y ` + pkg
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
