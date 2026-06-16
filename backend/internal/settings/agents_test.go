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

func TestEnsureAgentDefaultsReplacesLegacyCodexDefaultCommand(t *testing.T) {
	for _, legacy := range legacyCodexACPCommands {
		store, err := jsonstore.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		seed := testAgentDefaultsSeed()
		old := seed
		old.ACP = map[string]ACPAgentDefaults{
			"codex": {
				Enabled:         true,
				Command:         legacy,
				Model:           "gpt-5.5",
				ReasoningEffort: "medium",
			},
		}
		if _, err := SaveAgentDefaults(store, old); err != nil {
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
			t.Fatalf("codex command = %q, want upgraded from %q", loaded.ACP["codex"].Command, legacy)
		}
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

func TestEnsureAgentDefaultsUpgradesLegacyGrokCommand(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seed := testAgentDefaultsSeed()
	old := seed
	old.ACP = map[string]ACPAgentDefaults{
		"grok": {
			Enabled:         true,
			Command:         legacyGrokACPCommand,
			Model:           "grok-build",
			ReasoningEffort: "medium",
		},
	}
	if _, err := SaveAgentDefaults(store, old); err != nil {
		t.Fatal(err)
	}

	if err := EnsureAgentDefaults(store, seed); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAgentDefaults(store)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ACP["grok"].Command != seed.ACP["grok"].Command {
		t.Fatalf("grok command = %q", loaded.ACP["grok"].Command)
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

func TestMergeAgentDefaultsDropsLegacyGrokJazProfile(t *testing.T) {
	seed := testAgentDefaultsSeed()
	stored := AgentDefaults{
		Native: seed.Native,
		ACP:    map[string]ACPAgentDefaults{},
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
