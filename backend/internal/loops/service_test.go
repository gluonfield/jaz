package loops

import (
	"strings"
	"testing"
	"time"
)

func TestMetadataPromptUsesFreshRunMetadataOnly(t *testing.T) {
	runAt := time.Date(2026, 6, 8, 9, 30, 0, 0, time.UTC)
	prompt := MetadataPrompt(Loop{
		ID:              "loop-1",
		Name:            "Morning check",
		Prompt:          "Check overnight alerts.",
		MemoryPath:      "/tmp/jaz/automations/morning-check/memory.md",
		LastRunID:       "run-prev",
		LastRunThreadID: "thread-prev",
		LastRunStatus:   RunStatusOK,
		LastRunAt:       runAt.Add(-24 * time.Hour),
	}, Run{
		ID:           "run-now",
		ScheduledFor: runAt,
	}, runAt)

	for _, want := range []string{
		"Loop name: Morning check",
		"Run ID: run-now",
		"Memory file: /tmp/jaz/automations/morning-check/memory.md",
		"read the memory file if it exists",
		"create or update the memory file with concise Markdown",
		"thread_id=thread-prev",
		"Check overnight alerts.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("metadata prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "assistant said") {
		t.Fatalf("metadata prompt should not include previous transcript content:\n%s", prompt)
	}
}
