package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSystemPromptEndToEnd builds the full native prompt with every layer
// populated and pins the construction invariants: layer order, the skills
// catalog appearing exactly once at the end, and the ACP extension being
// byte-identical to the platform tail of the native prompt.
func TestSystemPromptEndToEnd(t *testing.T) {
	root := t.TempDir()
	memoryRoot := t.TempDir()
	write(t, root, "AGENTS.md", "always cite sources")
	write(t, root, "SOUL.md", "be direct")
	write(t, memoryRoot, "LONG_TERM.md", "- Goal: $5m through agent products.")
	write(t, memoryRoot, "SHORT_TERM.md", "- Focus: jaz memory system.")
	skillDir := filepath.Join(root, "skills", "deploy")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, skillDir, "SKILL.md", "---\nname: deploy\ndescription: ship & verify\n---\nsteps")

	builder := NewBuilder(root, filepath.Join(root, "workspaces", "default"), memoryRoot, func() bool { return true })
	system, err := builder.SystemPrompt()
	if err != nil {
		t.Fatal(err)
	}

	assertOrder(t, system,
		"You are Jaz",
		"Current working directory:",
		"agent_spawn only starts a session",
		"## Jaz platform",
		"## AGENTS.md\n\nalways cite sources",
		"## SOUL.md\n\nbe direct",
		"## Artifacts and visualizations",
		"Few-shot trace:",
		"## memory",
		"Capture as you go",
		"## memory/LONG_TERM.md\n\n- Goal: $5m through agent products.",
		"## memory/SHORT_TERM.md\n\n- Focus: jaz memory system.",
		"## Skills",
		"<name>deploy</name>",
		"<description>ship &amp; verify</description>",
	)
	for marker, want := range map[string]int{
		"## Jaz platform":     1,
		"<available_skills>":  1,
		"</available_skills>": 1,
		"## Skills":           1,
		"## AGENTS.md":        1,
		"## SOUL.md":          1,
		"Capture as you go":   1,
	} {
		if got := strings.Count(system, marker); got != want {
			t.Fatalf("%q must appear exactly %d time(s), got %d:\n%s", marker, want, got, system)
		}
	}

	acp, err := builder.ACPPrompt()
	if err != nil {
		t.Fatal(err)
	}
	if acp == "" || !strings.HasSuffix(system, acp) {
		t.Fatalf("the ACP extension must be the exact platform tail of the native prompt.\nACP:\n%s\nSYSTEM:\n%s", acp, system)
	}
	if strings.Contains(acp, "You are Jaz") || strings.Contains(acp, "agent_spawn") {
		t.Fatalf("acp extension must carry no coordinator identity or delegation rules:\n%s", acp)
	}
	if !strings.Contains(acp, "prefer an inline artifact over plain text") ||
		!strings.Contains(acp, "any reusable code snippet over 20 lines") ||
		!strings.Contains(acp, "plain lists, plain tables, enumerated content") ||
		!strings.Contains(acp, "visualize_show_widget") ||
		!strings.Contains(acp, "Never pass raw JSX, TSX, or an unbundled app to the render tool") {
		t.Fatalf("acp extension must carry the artifact policy:\n%s", acp)
	}

	// The master switch strips memory from both layers identically.
	disabledBuilder := NewBuilder(root, "", memoryRoot, func() bool { return false })
	disabledSystem, err := disabledBuilder.SystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	disabledACP, err := disabledBuilder.ACPPrompt()
	if err != nil {
		t.Fatal(err)
	}
	for _, prompt := range []string{disabledSystem, disabledACP} {
		if strings.Contains(prompt, "## memory") || strings.Contains(prompt, "Capture as you go") {
			t.Fatalf("disabled memory must vanish from every prompt:\n%s", prompt)
		}
	}
	if !strings.Contains(disabledACP, "<name>deploy</name>") {
		t.Fatalf("skills must survive the memory switch:\n%s", disabledACP)
	}
}
