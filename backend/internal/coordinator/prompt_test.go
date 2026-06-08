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
	write(t, root, "HEARTBEAT.md", "heartbeat")

	now := time.Date(2026, 6, 2, 9, 8, 7, 0, time.FixedZone("BST", 3600))
	workspace := filepath.Join(root, "workspaces", "default")
	prompt, err := prompt(root, workspace, "skills", now)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, prompt, "Date: June 2, 2026", "Time: 09:08:07 BST", "Timezone: BST (UTC+01:00)", "Weekday: Tuesday", "Current working directory: "+workspace, "~/.jaz: runtime state", "~/.jaz/workspaces/default: default tool cwd", "## AGENTS.md\n\nagents", "## SOUL.md\n\nsoul", "## HEARTBEAT.md\n\nheartbeat", "skills")
}

func TestPromptOmitsMissingFiles(t *testing.T) {
	prompt, err := Prompt(t.TempDir(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "## AGENTS.md") || strings.Contains(prompt, "## SOUL.md") || strings.Contains(prompt, "## HEARTBEAT.md") {
		t.Fatalf("prompt includes missing file sections:\n%s", prompt)
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
