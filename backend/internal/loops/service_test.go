package loops

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunSystemPromptUsesFreshRunMetadataOnly(t *testing.T) {
	memoryPath := filepath.Join(t.TempDir(), "memory.md")
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
	prompt := RunSystemPrompt(loop, Run{
		ID:           "run-now",
		ScheduledFor: runAt,
	}, runAt)

	for _, want := range []string{
		"Loop: Morning check (loop-1)",
		"Run: run-now scheduled 2026-06-08T09:30:00Z; now 2026-06-08T09:30:00Z",
		"Memory: " + memoryPath,
		"If the memory file exists, read it before starting.",
		"update memory with concise durable Markdown",
		"thread_id=thread-prev",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("run prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "assistant said") {
		t.Fatalf("run prompt should not include previous transcript content:\n%s", prompt)
	}
	if strings.Contains(prompt, "Check overnight alerts.") {
		t.Fatalf("run system prompt should not include the user task:\n%s", prompt)
	}
	if strings.Contains(prompt, "(new; do not read)") || strings.Contains(prompt, "(read before starting)") {
		t.Fatalf("run prompt must not branch on memory file existence:\n%s", prompt)
	}
}

func TestRunSystemPromptPutsExtrasAfterRules(t *testing.T) {
	runAt := time.Date(2026, 6, 8, 9, 30, 0, 0, time.UTC)
	prompt := RunSystemPrompt(Loop{
		ID:         "loop-1",
		Name:       "Morning check",
		Prompt:     "Check overnight alerts.",
		MemoryPath: "/tmp/jaz/automations/morning-check/memory.md",
	}, Run{ID: "run-now", ScheduledFor: runAt}, runAt,
		"## Widget\n\n- publish the widget",
	)

	widget := strings.Index(prompt, "## Widget")
	memory := strings.Index(prompt, "update memory with concise durable Markdown")
	if widget == -1 || memory == -1 {
		t.Fatalf("prompt missing sections (widget=%d memory=%d):\n%s", widget, memory, prompt)
	}
	if !(memory < widget) {
		t.Fatalf("prompt sections out of order (memory=%d widget=%d):\n%s", memory, widget, prompt)
	}
	if strings.Contains(prompt, "## Your task") || strings.Contains(prompt, "Check overnight alerts.") {
		t.Fatalf("system prompt must not include the user task:\n%s", prompt)
	}
}
