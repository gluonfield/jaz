package looprun

import (
	"strings"
	"testing"
)

func TestRenderOrdersContextExtrasThenTask(t *testing.T) {
	prompt, err := Render(Data{
		LoopName:     "memory-consolidation",
		LoopID:       "loop-1",
		RunID:        "run-9",
		ScheduledFor: "2026-06-11T23:30:00Z",
		Now:          "2026-06-11T23:30:02Z",
		MemoryPath:   "/tmp/automations/memory-consolidation/memory.md",
		PreviousRun:  `id=run-8 status=error error="dial tcp: timeout"`,
		Extras:       []string{"## Widget instructions\n\n- update the tile"},
		Prompt:       "Review yesterday's sessions.",
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := []string{
		"You are running a scheduled Jaz loop.",
		"Loop: memory-consolidation (id loop-1)",
		"Run ID: run-9",
		"Memory file: /tmp/automations/memory-consolidation/memory.md",
		`Previous run: id=run-8 status=error error="dial tcp: timeout"`,
		"read the memory file if it exists",
		"## Widget instructions",
		"## Your task",
		"Review yesterday's sessions.",
	}
	offset := 0
	for _, part := range parts {
		i := strings.Index(prompt[offset:], part)
		if i < 0 {
			t.Fatalf("missing %q in order in:\n%s", part, prompt)
		}
		offset += i + len(part)
	}
}

func TestRenderWithoutMemoryPathOmitsMemoryRules(t *testing.T) {
	prompt, err := Render(Data{LoopName: "n", LoopID: "l", RunID: "r", ScheduledFor: "s", Now: "n", PreviousRun: "none", Prompt: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "Memory file:") || strings.Contains(prompt, "read the memory file") {
		t.Fatalf("memory rules must be omitted without a memory path:\n%s", prompt)
	}
	if !strings.HasSuffix(strings.TrimSpace(prompt), "task") {
		t.Fatalf("the task must come last:\n%s", prompt)
	}
}
