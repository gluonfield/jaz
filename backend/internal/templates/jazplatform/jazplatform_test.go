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
		"## Inline artifacts",
		"Always call `visualize:read_me` before the first visual artifact",
		"Few-shot trace:",
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
	for _, want := range []string{
		"verify the data and choose the source/method before loading artifact guidance",
		"Pass `platform:\"mobile\"` for mobile targets, `platform:\"desktop\"` for desktop targets",
		"`visualize_read_me` when the provider exposes only underscore-safe native function names",
		"`visualize_show_widget` on underscore-safe native function-tool surfaces",
		"meaningful snake_case title",
		"Stacking millions into bars",
		"prefer an inline artifact over plain text",
		"Never pass raw JSX, TSX, or an unbundled app to the render tool",
		"`visualize:read_me` is the visual styling authority",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("artifact policy missing %q:\n%s", want, prompt)
		}
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
