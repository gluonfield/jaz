package loops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunPromptUsesFreshRunMetadataOnly(t *testing.T) {
	memoryPath := filepath.Join(t.TempDir(), "memory.md")
	if err := os.WriteFile(memoryPath, []byte("# notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	runAt := time.Date(2026, 6, 8, 9, 30, 0, 0, time.UTC)
	loop := Loop{
		ID:              "loop-1",
		Name:            "Morning check",
		Prompt:          "Check overnight alerts.",
		MemoryPath:      memoryPath,
		LastRunID:       "run-prev",
		LastRunThreadID: "thread-prev",
		LastRunStatus:   RunStatusOK,
		LastRunAt:       runAt.Add(-24 * time.Hour),
	}
	prompt := RunPrompt(loop, Run{
		ID:           "run-now",
		ScheduledFor: runAt,
	}, runAt)

	for _, want := range []string{
		"Loop: Morning check (id loop-1)",
		"Run ID: run-now",
		"Memory file: " + memoryPath,
		"Read the memory file before starting the task.",
		"create or update the memory file with concise Markdown",
		"thread_id=thread-prev",
		"Check overnight alerts.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("run prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "assistant said") {
		t.Fatalf("run prompt should not include previous transcript content:\n%s", prompt)
	}

	// A missing memory file is announced up front, not discovered via a failed read.
	loop.MemoryPath = filepath.Join(t.TempDir(), "memory.md")
	prompt = RunPrompt(loop, Run{ID: "run-now", ScheduledFor: runAt}, runAt)
	if !strings.Contains(prompt, "does not exist yet") {
		t.Fatalf("run prompt must announce a missing memory file:\n%s", prompt)
	}
}

func TestRunPromptPutsTaskLastAfterExtras(t *testing.T) {
	runAt := time.Date(2026, 6, 8, 9, 30, 0, 0, time.UTC)
	prompt := RunPrompt(Loop{
		ID:         "loop-1",
		Name:       "Morning check",
		Prompt:     "Check overnight alerts.",
		MemoryPath: "/tmp/jaz/automations/morning-check/memory.md",
	}, Run{ID: "run-now", ScheduledFor: runAt}, runAt,
		"## Widget instructions\n\n- publish the widget",
	)

	task := strings.Index(prompt, "## Your task")
	widget := strings.Index(prompt, "## Widget instructions")
	memory := strings.Index(prompt, "create or update the memory file")
	user := strings.Index(prompt, "Check overnight alerts.")
	if task == -1 || widget == -1 || memory == -1 || user == -1 {
		t.Fatalf("prompt missing sections (task=%d widget=%d memory=%d user=%d):\n%s", task, widget, memory, user, prompt)
	}
	// Instructions first, widget extras next, the user's task last.
	if !(memory < widget && widget < task && task < user) {
		t.Fatalf("prompt sections out of order (memory=%d widget=%d task=%d user=%d):\n%s", memory, widget, task, user, prompt)
	}
	if !strings.HasSuffix(strings.TrimSpace(prompt), "Check overnight alerts.") {
		t.Fatalf("user prompt is not the final content:\n%s", prompt)
	}
}
