package coordinator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/connections"
	"github.com/wins/jaz/backend/internal/sessioncontext"
	"github.com/wins/jaz/backend/internal/visualize"
)

func TestBuilderPicksUpEditsWithoutRestart(t *testing.T) {
	root := t.TempDir()
	builder := NewBuilder(root, filepath.Join(root, "workspaces", "default"), "", nil)

	prompt, err := builder.SystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "## SOUL.md\n\n(empty)") || !strings.Contains(prompt, "## INTERNAL.md") {
		t.Fatalf("empty root should render SOUL.md as (empty):\n%s", prompt)
	}

	// Files created after the builder exists must appear on the next call.
	write(t, root, "SOUL.md", "be kind")
	skillDir := filepath.Join(root, "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, skillDir, "SKILL.md", "---\nname: deploy\ndescription: ship it\n---\nsteps")
	localSkillDir := filepath.Join(root, "workspaces", "default", ".codex", "skills", "test-local")
	if err := os.MkdirAll(localSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, localSkillDir, "SKILL.md", "---\nname: test-local\ndescription: local task\n---\nsteps")

	prompt, err = builder.SystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "be kind") {
		t.Fatalf("SOUL.md edit not picked up:\n%s", prompt)
	}
	if !strings.Contains(prompt, "<name>deploy</name>") {
		t.Fatalf("new skill not picked up:\n%s", prompt)
	}
	if !strings.Contains(prompt, "<name>test-local</name>") {
		t.Fatalf("local skill not picked up:\n%s", prompt)
	}
	skillsPrompt, err := builder.SkillsPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skillsPrompt, "<name>deploy</name>") || !strings.Contains(skillsPrompt, "<name>test-local</name>") {
		t.Fatal("skills prompt missing new skill")
	}

	// And edits to existing files replace the old content.
	write(t, root, "SOUL.md", "be bold")
	prompt, err = builder.SystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "be bold") || strings.Contains(prompt, "be kind") {
		t.Fatalf("SOUL.md rewrite not picked up:\n%s", prompt)
	}
}

func TestBuilderReturnsPromptReadErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "AGENTS.md"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := NewBuilder(root, "", "", nil).SystemPrompt()
	if err == nil {
		t.Fatal("expected prompt read error")
	}
}

// The ACP prompt extends the agent's own system prompt, so it must carry the
// user's rules, memory, and skills — but never the coordinator identity or
// persona files.
func TestBuilderACPPrompt(t *testing.T) {
	root := t.TempDir()
	memoryRoot := t.TempDir()
	write(t, root, "AGENTS.md", "save durable facts with jazmem")
	write(t, root, "SOUL.md", "be kind")
	write(t, root, "INTERNAL.md", "prefer direct fixes")
	write(t, memoryRoot, "LONG_TERM.md", "- Goal: $5m.")
	skillDir := filepath.Join(root, "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, skillDir, "SKILL.md", "---\nname: deploy\ndescription: ship it\n---\nsteps")

	cwd := filepath.Join(root, ".worktrees", "agent-task")
	localSkillDir := filepath.Join(cwd, ".agents", "skills", "local")
	if err := os.MkdirAll(localSkillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, localSkillDir, "SKILL.md", "---\nname: local\ndescription: cwd skill\n---\nsteps")
	prompt, err := NewBuilder(root, "", memoryRoot, func() bool { return true }).WithAgents(staticAgentNames{names: []string{"codex"}}).ACPPrompt(cwd)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Date: ",
		"Time: ",
		"Timezone: ",
		"Weekday: ",
		"Now: ",
		"Current working directory: " + cwd,
		"## Runtime paths",
		filepath.Join(root, "workspaces", "default", ".worktrees"),
		"## AGENTS.md",
		"save durable facts with jazmem",
		"## SOUL.md",
		"be kind",
		"## INTERNAL.md",
		"prefer direct fixes",
		"configured ACP agents: `codex`",
		"## memory/LONG_TERM.md",
		"- Goal: $5m.",
		"<name>deploy</name>",
		"<name>local</name>",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("acp prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Session context") {
		t.Fatalf("acp prompt should not spend tokens on a session context heading:\n%s", prompt)
	}
	if strings.Contains(prompt, "You are Jaz") {
		t.Fatalf("acp prompt must not contain the coordinator identity:\n%s", prompt)
	}
	mobilePrompt, err := NewBuilder(root, "", memoryRoot, func() bool { return true }).WithAgents(staticAgentNames{names: []string{"codex"}}).ACPPromptForContext(sessioncontext.WithClientPlatform(context.Background(), "mobile"), cwd, string(visualize.SurfaceChat))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(mobilePrompt, "Device: Mobile") {
		t.Fatalf("mobile acp prompt missing device line:\n%s", mobilePrompt)
	}

	disabled, err := NewBuilder(root, "", memoryRoot, func() bool { return false }).ACPPrompt(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(disabled, "## memory/") {
		t.Fatalf("disabled memory must not be injected:\n%s", disabled)
	}

	empty, err := NewBuilder(t.TempDir(), "", "", nil).ACPPrompt("")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(empty, "## AGENTS.md\n\n(empty)") || !strings.Contains(empty, "## SOUL.md\n\n(empty)") || !strings.Contains(empty, "## INTERNAL.md") {
		t.Fatalf("empty root must still render the platform with (empty) files:\n%s", empty)
	}
}

func TestBuilderSkipsMemoryWhenDisabled(t *testing.T) {
	root := t.TempDir()
	memoryRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(memoryRoot, "LONG_TERM.md"), []byte("# Long Term Memory\n\n- Goal: $5m."), 0o644); err != nil {
		t.Fatal(err)
	}

	enabled, err := NewBuilder(root, "", memoryRoot, func() bool { return true }).SystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(enabled, "## memory/LONG_TERM.md") {
		t.Fatalf("enabled builder should inject memory:\n%s", enabled)
	}

	disabled, err := NewBuilder(root, "", memoryRoot, func() bool { return false }).SystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(disabled, "## memory/") {
		t.Fatalf("disabled builder must not inject memory:\n%s", disabled)
	}
}

func TestBuilderIncludesConnections(t *testing.T) {
	builder := NewBuilder(t.TempDir(), "", t.TempDir(), func() bool { return true }).WithConnections(staticConnections{connections: []connections.AgentConnection{{
		ProviderName: "WhatsApp",
		Account:      "personal (+447700900123)",
		RelevantPaths: []connections.AgentPath{{
			Path:        "sources/chat/whatsapp/447700900123/contacts.md",
			Kind:        connections.AgentPathKindMemoryPage,
			Explanation: "Clean contacts.",
		}},
	}}})
	prompt, err := builder.ACPPrompt("")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"## connections",
		"WhatsApp: personal (+447700900123)",
		"`sources/chat/whatsapp/447700900123/contacts.md` (memory_page)",
		"Clean contacts.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing connection detail %q:\n%s", want, prompt)
		}
	}
}

func TestBuilderSkipsConnectionsWhenMemoryDisabled(t *testing.T) {
	builder := NewBuilder(t.TempDir(), "", t.TempDir(), func() bool { return false }).WithConnections(staticConnections{connections: []connections.AgentConnection{{
		ProviderName: "WhatsApp",
		Account:      "personal (+447700900123)",
		RelevantPaths: []connections.AgentPath{{
			Path:        "sources/chat/whatsapp/447700900123/contacts.md",
			Kind:        connections.AgentPathKindMemoryPage,
			Explanation: "Clean contacts.",
		}},
	}}})
	prompt, err := builder.ACPPrompt("")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "## connections") || strings.Contains(prompt, "sources/chat/whatsapp") {
		t.Fatalf("disabled memory must omit memory-backed connection hints:\n%s", prompt)
	}
}

func TestBuilderOmitsDelegationWhenAgentNamesFail(t *testing.T) {
	builder := NewBuilder(t.TempDir(), "", "", nil).WithAgents(failingAgentNames{})
	prompt, err := builder.ACPPrompt("")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "## Agent delegation") {
		t.Fatalf("prompt should omit best-effort delegation context after agent-name failure:\n%s", prompt)
	}
}

type staticConnections struct {
	connections []connections.AgentConnection
}

func (s staticConnections) AgentConnections(context.Context) ([]connections.AgentConnection, error) {
	return s.connections, nil
}

type staticAgentNames struct {
	names []string
}

func (s staticAgentNames) EnabledAgentNames() ([]string, error) {
	return s.names, nil
}

type failingAgentNames struct{}

func (failingAgentNames) EnabledAgentNames() ([]string, error) {
	return nil, errors.New("agent settings unavailable")
}
