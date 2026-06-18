package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSystemPromptEndToEnd builds the full coordinator prompt with every layer
// populated and pins the construction invariants: layer order, the skills
// catalog appearing exactly once at the end, and the ACP extension sharing the
// prompt-file/memory/skills tail of the coordinator prompt.
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
		"agent_spawn only starts a session",
		"## Jaz platform",
		"Current working directory:",
		"## AGENTS.md\n\nalways cite sources",
		"## SOUL.md\n\nbe direct",
		"## Artifacts and visualisation",
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
		"Date: ":              1,
		"Time: ":              1,
		"Timezone: ":          1,
		"Weekday: ":           1,
		"Now: ":               1,
		"## AGENTS.md":        1,
		"## SOUL.md":          1,
		"Capture as you go":   1,
	} {
		if got := strings.Count(system, marker); got != want {
			t.Fatalf("%q must appear exactly %d time(s), got %d:\n%s", marker, want, got, system)
		}
	}

	acpCwd := filepath.Join(root, "workspaces", "default", ".worktrees", "agent-task")
	acp, err := builder.ACPPrompt(acpCwd)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(acp, "Current working directory: "+acpCwd) {
		t.Fatalf("acp extension must carry the resolved session cwd:\n%s", acp)
	}
	for _, want := range []string{"Date: ", "Time: ", "Timezone: ", "Weekday: ", "Now: "} {
		if !strings.Contains(acp, want) {
			t.Fatalf("acp extension missing runtime context %q:\n%s", want, acp)
		}
	}
	sharedOffset := strings.Index(acp, "## AGENTS.md")
	if sharedOffset < 0 {
		t.Fatalf("acp extension missing shared prompt-file tail:\n%s", acp)
	}
	if !strings.HasSuffix(system, acp[sharedOffset:]) {
		t.Fatalf("the ACP shared tail must match the coordinator shared tail.\nACP:\n%s\nSYSTEM:\n%s", acp, system)
	}
	if strings.Contains(acp, "You are Jaz") || strings.Contains(acp, "agent_spawn") {
		t.Fatalf("acp extension must carry no coordinator identity or delegation rules:\n%s", acp)
	}
	if !strings.Contains(acp, "prefer an inline artifact over plain text") ||
		!strings.Contains(acp, "any reusable code snippet over 20 lines") ||
		!strings.Contains(acp, "plain lists, plain tables, enumerated content") ||
		!strings.Contains(acp, "visualise_show_widget") ||
		!strings.Contains(acp, "Never pass raw JSX, TSX, or an unbundled app to the render tool") {
		t.Fatalf("acp extension must carry the artifact policy:\n%s", acp)
	}
	for _, reject := range []string{"`visualize:", "visualize_", "`create_file`", "file-artifact tool", "coding-agent surface provides", "Claude-compatible"} {
		if strings.Contains(acp, reject) {
			t.Fatalf("artifact policy must use direct Jaz visualise tools only; found %q:\n%s", reject, acp)
		}
	}
	for _, reject := range []string{"visualise:show_widget", "visualise:read_me"} {
		if strings.Contains(acp, reject) {
			t.Fatalf("artifact policy must use identifier-safe tool names; found %q:\n%s", reject, acp)
		}
	}

	widgetACP, err := builder.ACPPromptForArtifactSurface(acpCwd, "widget")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(widgetACP, "## Artifacts and visualisation") ||
		strings.Contains(widgetACP, "visualise_show_widget") ||
		strings.Contains(widgetACP, "visualise_read_me") {
		t.Fatalf("widget ACP extension must not carry the chat artifact policy:\n%s", widgetACP)
	}
	for _, want := range []string{"## AGENTS.md\n\nalways cite sources", "## memory/LONG_TERM.md", "<name>deploy</name>"} {
		if !strings.Contains(widgetACP, want) {
			t.Fatalf("widget ACP extension missing shared section %q:\n%s", want, widgetACP)
		}
	}

	// The master switch strips memory from both layers identically.
	disabledBuilder := NewBuilder(root, "", memoryRoot, func() bool { return false })
	disabledSystem, err := disabledBuilder.SystemPrompt()
	if err != nil {
		t.Fatal(err)
	}
	disabledACP, err := disabledBuilder.ACPPrompt("")
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
