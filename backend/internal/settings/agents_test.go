package settings

import (
	"encoding/json"
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
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seed := testAgentDefaultsSeed()
	old := seed
	old.ACP = map[string]ACPAgentDefaults{
		"codex": {
			Enabled:         true,
			Command:         legacyCodexACPCommand,
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
		t.Fatalf("codex command = %q", loaded.ACP["codex"].Command)
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

func TestEnsureAgentDefaultsUpgradesPreviousClaudeCommands(t *testing.T) {
	for _, legacy := range legacyClaudeCodeCommands {
		store, err := jsonstore.New(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		seed := testAgentDefaultsSeed()
		old := seed
		old.ACP = map[string]ACPAgentDefaults{
			"claude": {
				Enabled: true,
				Command: legacy,
				Model:   "default",
			},
		}
		data, err := json.Marshal(old)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.SaveSetting(AgentSettingsNamespace, AgentDefaultsKey, data); err != nil {
			t.Fatal(err)
		}

		if err := EnsureAgentDefaults(store, seed); err != nil {
			t.Fatal(err)
		}
		loaded, err := LoadAgentDefaults(store)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.ACP["claude"].Command != seed.ACP["claude"].Command {
			t.Fatalf("claude command = %q, want upgraded from %q", loaded.ACP["claude"].Command, legacy)
		}
	}
}

func TestEnsureAgentDefaultsMigratesLegacyClaudeCodeModel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seed := testAgentDefaultsSeed()
	legacyClaudeName := strings.ReplaceAll("claude-code", "-", "_")
	old := seed
	old.ACP = map[string]ACPAgentDefaults{
		legacyClaudeName: {
			Enabled:         true,
			Command:         legacyClaudeCodeCommands[0],
			Model:           legacyClaudeCodeModel,
			ReasoningEffort: "xhigh",
		},
	}
	data, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveSetting(AgentSettingsNamespace, AgentDefaultsKey, data); err != nil {
		t.Fatal(err)
	}

	if err := EnsureAgentDefaults(store, seed); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadAgentDefaults(store)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ACP["claude"].Model != "default" {
		t.Fatalf("claude model = %q", loaded.ACP["claude"].Model)
	}
	if loaded.ACP["claude"].Command != seed.ACP["claude"].Command {
		t.Fatalf("claude command = %q", loaded.ACP["claude"].Command)
	}
	if loaded.ACP["claude"].ReasoningEffort != "xhigh" {
		t.Fatalf("claude effort = %q", loaded.ACP["claude"].ReasoningEffort)
	}
	if _, ok := loaded.ACP[legacyClaudeName]; ok {
		t.Fatalf("legacy claude key was not migrated: %#v", loaded.ACP)
	}
}
