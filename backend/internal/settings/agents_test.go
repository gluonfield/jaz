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

func TestEnsureAgentDefaultsKeepsCustomCodexCommand(t *testing.T) {
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
	if loaded.ACP["codex"].Command != "/custom/codex-acp" {
		t.Fatalf("codex command was overwritten: %q", loaded.ACP["codex"].Command)
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
	codex.Command = strings.Replace(codex.Command, "@jazchat/codex-acp@0.16.7", "@jazchat/codex-acp@0.16.1", 1)
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
	if loaded.ACP["codex"].Command != seed.ACP["codex"].Command {
		t.Fatalf("codex command = %q, want %q", loaded.ACP["codex"].Command, seed.ACP["codex"].Command)
	}
}

func TestMergeAgentDefaultsRefreshesPreviousCodexWindowsCommand(t *testing.T) {
	seed := AgentDefaults{ACP: map[string]ACPAgentDefaults{
		"codex": {
			Command: `npx.cmd -y @jazchat/codex-acp@0.16.7 -c 'sandbox_mode="danger-full-access"' -c 'approval_policy="never"' -c features.tool_search_always_defer_mcp_tools=true -c suppress_unstable_features_warning=true`,
		},
	}}
	previousPackage := "@jazchat/codex-acp@0.16.6"
	for _, storedCommand := range []string{
		strings.Replace(seed.ACP["codex"].Command, "@jazchat/codex-acp@0.16.7", previousPackage, 1),
		strings.Replace(strings.Replace(seed.ACP["codex"].Command, "@jazchat/codex-acp@0.16.7", previousPackage, 1), " -c suppress_unstable_features_warning=true", "", 1),
	} {
		stored := AgentDefaults{ACP: map[string]ACPAgentDefaults{
			"codex": {Command: storedCommand},
		}}

		merged := MergeAgentDefaults(stored, seed, []string{"codex"})

		if merged.ACP["codex"].Command != seed.ACP["codex"].Command {
			t.Fatalf("codex command = %q, want %q", merged.ACP["codex"].Command, seed.ACP["codex"].Command)
		}
	}
}

func TestMergeAgentDefaultsRefreshesCurrentCodexCommandMissingWarningSuppress(t *testing.T) {
	seed := testAgentDefaultsSeed()
	storedCommand := strings.Replace(seed.ACP["codex"].Command, " -c suppress_unstable_features_warning=true", "", 1)
	stored := AgentDefaults{ACP: map[string]ACPAgentDefaults{
		"codex": {Command: storedCommand},
	}}

	merged := MergeAgentDefaults(stored, seed, []string{"codex"})

	if merged.ACP["codex"].Command != seed.ACP["codex"].Command {
		t.Fatalf("codex command = %q, want %q", merged.ACP["codex"].Command, seed.ACP["codex"].Command)
	}
}

func TestMergeAgentDefaultsKeepsFutureCodexPackage(t *testing.T) {
	seed := testAgentDefaultsSeed()
	storedCommand := strings.Replace(seed.ACP["codex"].Command, "@jazchat/codex-acp@0.16.7", "@jazchat/codex-acp@0.16.8", 1)
	stored := AgentDefaults{ACP: map[string]ACPAgentDefaults{
		"codex": {Command: storedCommand},
	}}

	merged := MergeAgentDefaults(stored, seed, []string{"codex"})

	if merged.ACP["codex"].Command != storedCommand {
		t.Fatalf("codex command = %q, want custom future package %q", merged.ACP["codex"].Command, storedCommand)
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

	if merged.ACP["codex"].Command != seed.ACP["codex"].Command {
		t.Fatalf("codex command = %q, want %q", merged.ACP["codex"].Command, seed.ACP["codex"].Command)
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
