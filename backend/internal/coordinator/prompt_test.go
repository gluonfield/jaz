package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPromptCombinesCoordinatorFiles(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "agents")
	write(t, root, "SOUL.md", "soul")

	now := time.Date(2026, 6, 2, 9, 8, 7, 0, time.FixedZone("BST", 3600))
	workspace := filepath.Join(root, "workspaces", "default")
	prompt, err := prompt(root, workspace, "", "skills", now)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, prompt, "Date: June 2, 2026", "Time: 09:08:07 BST", "Timezone: BST (UTC+01:00)", "Weekday: Tuesday", "Current working directory: "+workspace, "~/.jaz: runtime state", "~/.jaz/workspaces/default: default tool cwd", "## AGENTS.md\n\nagents", "## SOUL.md\n\nsoul", "skills")
}

func TestPromptOmitsMissingFiles(t *testing.T) {
	prompt, err := Prompt(t.TempDir(), "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "## AGENTS.md") || strings.Contains(prompt, "## SOUL.md") {
		t.Fatalf("prompt includes missing file sections:\n%s", prompt)
	}
}

func TestPromptIgnoresHeartbeatFile(t *testing.T) {
	root := t.TempDir()
	write(t, root, "HEARTBEAT.md", "heartbeat")

	prompt, err := Prompt(root, "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "HEARTBEAT.md") || strings.Contains(prompt, "heartbeat") {
		t.Fatalf("prompt includes retired heartbeat file:\n%s", prompt)
	}
}

func write(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertOrder(t *testing.T, value string, parts ...string) {
	t.Helper()
	offset := 0
	for _, part := range parts {
		i := strings.Index(value[offset:], part)
		if i < 0 {
			t.Fatalf("missing %q in:\n%s", part, value)
		}
		offset += i + len(part)
	}
}

func TestPromptInjectsMemoryHorizons(t *testing.T) {
	root := t.TempDir()
	memoryRoot := t.TempDir()
	write(t, root, "AGENTS.md", "agents")
	write(t, memoryRoot, "LONG_TERM.md", "# Long Term Memory\n\n- Goal: $5m through agent products.")
	write(t, memoryRoot, "SHORT_TERM.md", "# Short Term Memory\n\n- Focus: jaz memory system.")
	if err := os.MkdirAll(filepath.Join(memoryRoot, "daily"), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	today := now.Local().Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Local().Format("2006-01-02")
	write(t, memoryRoot, "daily/"+today+".md", "# Daily\n\n- shipped provenance fields")
	write(t, memoryRoot, "daily/"+yesterday+".md", "# Daily\n\n- reviewed gbrain")

	got, err := prompt(root, "", memoryRoot, "", now)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, got,
		"## AGENTS.md",
		"## memory/LONG_TERM.md", "$5m through agent products",
		"## memory/SHORT_TERM.md", "jaz memory system",
		"## memory/daily/"+today+".md", "shipped provenance fields",
		"## memory/daily/"+yesterday+".md", "reviewed gbrain",
	)

	missing, err := prompt(root, "", t.TempDir(), "", now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(missing, "## memory/") {
		t.Fatalf("missing memory files must not add sections:\n%s", missing)
	}
}
