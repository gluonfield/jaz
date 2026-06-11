package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuilderPicksUpEditsWithoutRestart(t *testing.T) {
	root := t.TempDir()
	builder := NewBuilder(root, filepath.Join(root, "workspaces", "default"), "", nil)

	prompt, err := builder.SystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "## SOUL.md") {
		t.Fatalf("empty root should have no SOUL.md section:\n%s", prompt)
	}

	// Files created after the builder exists must appear on the next call.
	write(t, root, "SOUL.md", "be kind")
	skillDir := filepath.Join(root, "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, skillDir, "SKILL.md", "---\nname: deploy\ndescription: ship it\n---\nsteps")

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
	skillsPrompt, err := builder.SkillsPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(skillsPrompt, "<name>deploy</name>") {
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
