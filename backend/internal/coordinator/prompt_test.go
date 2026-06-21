package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/visualize"
)

func TestPromptCombinesCoordinatorFiles(t *testing.T) {
	root := t.TempDir()
	write(t, root, "AGENTS.md", "agents")
	write(t, root, "SOUL.md", "soul")

	now := time.Date(2026, 6, 2, 9, 8, 7, 0, time.FixedZone("BST", 3600))
	workspace := filepath.Join(root, "workspaces", "default")
	prompt, err := prompt(root, workspace, "", "skills", visualize.SurfaceChat, now)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, prompt, root+": runtime state", workspace+": default tool cwd", "## Jaz platform", "Date: June 2, 2026", "Time: 09:08:07 BST", "Timezone: BST (UTC+01:00)", "Weekday: Tuesday", "Current working directory: "+workspace, "## AGENTS.md\n\nagents", "## SOUL.md\n\nsoul", "skills")
}

func TestPromptOmitsMissingFiles(t *testing.T) {
	prompt, err := Prompt(t.TempDir(), "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "## AGENTS.md\n\n(empty)") || !strings.Contains(prompt, "## SOUL.md\n\n(empty)") {
		t.Fatalf("missing files must render as (empty) sections:\n%s", prompt)
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
	write(t, memoryRoot, "daily/"+today+".md", "# Daily\n\n- shipped provenance fields")

	got, err := prompt(root, "", memoryRoot, "", visualize.SurfaceChat, now)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, got,
		"## Jaz platform",
		"## AGENTS.md",
		"## memory\n", "Capture as you go",
		"## memory/LONG_TERM.md", "$5m through agent products",
		"## memory/SHORT_TERM.md", "jaz memory system",
		"## memory/daily/"+today+".md", "shipped provenance fields",
	)

	missing, err := prompt(root, "", t.TempDir(), "", visualize.SurfaceChat, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(missing, "## memory/LONG_TERM.md\n\n(empty)") || !strings.Contains(missing, "## memory/SHORT_TERM.md\n\n(empty)") {
		t.Fatalf("horizons must always render when memory is enabled:\n%s", missing)
	}
	if strings.Contains(missing, "## memory/daily/") {
		t.Fatalf("absent daily pages must not add sections:\n%s", missing)
	}
	if !strings.Contains(missing, "Capture as you go") {
		t.Fatalf("memory protocol should inject whenever memory is enabled:\n%s", missing)
	}

	disabled, err := prompt(root, "", "", "", visualize.SurfaceChat, now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(disabled, "Capture as you go") {
		t.Fatalf("disabled memory must not inject the protocol:\n%s", disabled)
	}
}

func TestPromptDoesNotTruncateMemorySections(t *testing.T) {
	root := t.TempDir()
	memoryRoot := t.TempDir()
	write(t, root, "AGENTS.md", "agents")
	longTerm := "# Long Term Memory\n\n" + strings.Repeat("l", 3000) + " long-tail"
	shortTerm := "# Short Term Memory\n\n" + strings.Repeat("s", 2000) + " short-tail"
	write(t, memoryRoot, "LONG_TERM.md", longTerm)
	write(t, memoryRoot, "SHORT_TERM.md", shortTerm)
	if err := os.MkdirAll(filepath.Join(memoryRoot, "daily"), 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	today := now.Local().Format("2006-01-02")
	daily := "# Daily\n\n" + strings.Repeat("d", 1600) + " daily-tail"
	write(t, memoryRoot, "daily/"+today+".md", daily)

	got, err := prompt(root, "", memoryRoot, "", visualize.SurfaceChat, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"<truncated after", "<truncated before"} {
		if !strings.Contains(got, marker) {
			continue
		}
		t.Fatalf("memory prompt must not contain truncation marker %q in:\n%s", marker, got)
	}
	for _, want := range []string{"long-tail", "short-tail", "daily-tail"} {
		if !strings.Contains(got, want) {
			t.Fatalf("memory prompt missing untruncated content %q in:\n%s", want, got)
		}
	}
}
