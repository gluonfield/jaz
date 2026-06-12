package jazplatform

import (
	"strings"
	"testing"
)

func TestRenderNamesEverySurfaceExplicitly(t *testing.T) {
	prompt, err := Render(Data{
		Agents: "agents",
		Soul:   "soul",
		Memory: &MemoryData{
			LongTerm:  "- Goal: $5m.",
			ShortTerm: "- Focus: jaz memory.",
			TodayName: "daily/2026-06-11.md",
			Today:     "- shipped templates",
		},
		Skills: "skills-block",
	})
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, prompt,
		"## Jaz platform",
		"## AGENTS.md\n\nagents",
		"## SOUL.md\n\nsoul",
		"## memory",
		"broad context from the user's past behavior",
		"start from the user's question",
		"Capture as you go",
		"## memory/LONG_TERM.md\n\n- Goal: $5m.",
		"## memory/SHORT_TERM.md\n\n- Focus: jaz memory.",
		"## memory/daily/2026-06-11.md\n\n- shipped templates",
		"skills-block",
	)
	if strings.Contains(prompt, "You are Jaz") {
		t.Fatalf("platform prompt must not carry the coordinator identity:\n%s", prompt)
	}
}

func TestRenderMemoryStates(t *testing.T) {
	disabled, err := Render(Data{Agents: "agents", Soul: "(empty)"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(disabled, "## memory") || strings.Contains(disabled, "Capture as you go") {
		t.Fatalf("nil memory must omit the memory block:\n%s", disabled)
	}
	if !strings.Contains(disabled, "## AGENTS.md") || !strings.Contains(disabled, "## SOUL.md") {
		t.Fatalf("prompt files must render regardless of memory:\n%s", disabled)
	}

	fresh, err := Render(Data{Agents: "(empty)", Soul: "(empty)", Memory: &MemoryData{LongTerm: "(empty)", ShortTerm: "(empty)"}})
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, fresh, "Capture as you go", "## memory/LONG_TERM.md\n\n(empty)", "## memory/SHORT_TERM.md\n\n(empty)")
	if strings.Contains(fresh, "## memory/daily/") {
		t.Fatalf("no daily content means no daily sections:\n%s", fresh)
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
