package settings

import (
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func testAgentDefaultsSeed() AgentDefaults {
	return AgentDefaultsFromCatalog(NativeAgentDefaults{}, acp.BuiltinAgents())
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

func TestEnsureAgentDefaultsMigratesLegacyClaudeCodeModel(t *testing.T) {
	store, err := jsonstore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seed := testAgentDefaultsSeed()
	old := seed
	old.ACP = map[string]ACPAgentDefaults{
		"claude_code": {
			Enabled:         true,
			Command:         seed.ACP["claude_code"].Command,
			Model:           legacyClaudeCodeModel,
			ReasoningEffort: "xhigh",
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
	if loaded.ACP["claude_code"].Model != "default" {
		t.Fatalf("claude_code model = %q", loaded.ACP["claude_code"].Model)
	}
	if loaded.ACP["claude_code"].ReasoningEffort != "xhigh" {
		t.Fatalf("claude_code effort = %q", loaded.ACP["claude_code"].ReasoningEffort)
	}
}
