package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuilderPicksUpEditsWithoutRestart(t *testing.T) {
	root := t.TempDir()
	builder := NewBuilder(root, filepath.Join(root, "workspaces", "default"), nil)

	if prompt := builder.SystemPrompt(); strings.Contains(prompt, "## SOUL.md") {
		t.Fatalf("empty root should have no SOUL.md section:\n%s", prompt)
	}

	// Files created after the builder exists must appear on the next call.
	write(t, root, "SOUL.md", "be kind")
	skillDir := filepath.Join(root, "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, skillDir, "SKILL.md", "---\nname: deploy\ndescription: ship it\n---\nsteps")

	prompt := builder.SystemPrompt()
	if !strings.Contains(prompt, "be kind") {
		t.Fatalf("SOUL.md edit not picked up:\n%s", prompt)
	}
	if !strings.Contains(prompt, "<name>deploy</name>") {
		t.Fatalf("new skill not picked up:\n%s", prompt)
	}
	if !strings.Contains(builder.SkillsPrompt(), "<name>deploy</name>") {
		t.Fatal("skills prompt missing new skill")
	}

	// And edits to existing files replace the old content.
	write(t, root, "SOUL.md", "be bold")
	if prompt := builder.SystemPrompt(); !strings.Contains(prompt, "be bold") || strings.Contains(prompt, "be kind") {
		t.Fatalf("SOUL.md rewrite not picked up:\n%s", prompt)
	}
}

func TestBuilderFallsBackToLastGoodBuild(t *testing.T) {
	root := t.TempDir()
	write(t, root, "SOUL.md", "soul")
	builder := NewBuilder(root, filepath.Join(root, "workspaces", "default"), nil)
	if !strings.Contains(builder.SystemPrompt(), "soul") {
		t.Fatal("initial build missing SOUL.md")
	}

	// An unreadable skills tree fails the rebuild; the last good prompt
	// must survive instead of vanishing mid-conversation.
	blocked := filepath.Join(root, "skills", "blocked")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	if !strings.Contains(builder.SystemPrompt(), "soul") {
		t.Fatal("fallback prompt lost after rebuild failure")
	}
}
