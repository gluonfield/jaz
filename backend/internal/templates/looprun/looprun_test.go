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
		Extras:       []string{"## Widget\n\n- update the tile"},
		Prompt:       "Review yesterday's sessions.",
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := []string{
		"Scheduled Jaz loop run.",
		"Loop: memory-consolidation (loop-1)",
		"Run: run-9 scheduled 2026-06-11T23:30:00Z; now 2026-06-11T23:30:02Z",
		"Memory: /tmp/automations/memory-consolidation/memory.md",
		`Previous: id=run-8 status=error error="dial tcp: timeout"`,
		"If the memory file exists, read it before starting.",
		"## Widget",
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

func TestRenderMemoryUsesSingleInvariant(t *testing.T) {
	prompt, err := Render(Data{
		LoopName: "n", LoopID: "l", RunID: "r", ScheduledFor: "s", Now: "n",
		MemoryPath:  "/tmp/automations/n/memory.md",
		PreviousRun: "none", Prompt: "task",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Memory: /tmp/automations/n/memory.md",
		"If the memory file exists, read it before starting.",
		"update memory with concise durable Markdown",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("memory prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, reject := range []string{"(read before starting)", "(new; do not read)"} {
		if strings.Contains(prompt, reject) {
			t.Fatalf("memory prompt must not branch on file existence; found %q:\n%s", reject, prompt)
		}
	}
}

func TestRenderWithoutMemoryPathOmitsMemoryRules(t *testing.T) {
	prompt, err := Render(Data{LoopName: "n", LoopID: "l", RunID: "r", ScheduledFor: "s", Now: "n", PreviousRun: "none", Prompt: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(prompt, "Memory:") || strings.Contains(prompt, "memory file directory") {
		t.Fatalf("memory rules must be omitted without a memory path:\n%s", prompt)
	}
	if !strings.HasSuffix(strings.TrimSpace(prompt), "task") {
		t.Fatalf("the task must come last:\n%s", prompt)
	}
}
