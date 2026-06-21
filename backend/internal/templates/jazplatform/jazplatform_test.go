package jazplatform

import (
	"strings"
	"testing"
)

func TestRenderNamesEverySurfaceExplicitly(t *testing.T) {
	prompt, err := Render(Data{
		Agents:   "agents",
		Date:     "June 16, 2026",
		Time:     "12:34:56 BST",
		Timezone: "BST (UTC+01:00)",
		Weekday:  "Tuesday",
		Human:    "Tuesday, June 16, 2026 at 12:34:56 BST",
		Cwd:      "/tmp/jaz/workspaces/default/.worktrees/task",
		Soul:     "soul",
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
	prompt = normalizeNewlines(prompt)
	assertOrder(t, prompt,
		"## Jaz platform",
		"Date: June 16, 2026",
		"Time: 12:34:56 BST",
		"Timezone: BST (UTC+01:00)",
		"Weekday: Tuesday",
		"Now: Tuesday, June 16, 2026 at 12:34:56 BST",
		"Current working directory: /tmp/jaz/workspaces/default/.worktrees/task",
		"## AGENTS.md\n\nagents",
		"## SOUL.md\n\nsoul",
		"## Agent delegation",
		"Do not use an agent-local multi-agent tool",
		"## Artifacts and visualisation",
		"Artifact usage criteria:",
		"Always call `visualise_read_me` before the first artifact",
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
		"these govern behavior on the Jaz platform",
		"Launching background work is not delivery",
		"When Jaztools exposes `agent_spawn`",
		"Choose the agent with `acp_agent` or `agent_name`",
		"any reusable code snippet over 20 lines",
		"standalone text-heavy documents over 20 lines or 1500 characters",
		"Do not use artifacts for short code answers of 20 lines or fewer",
		"plain lists, plain tables, enumerated content",
		"Create single-file artifacts unless the user asks otherwise",
		"do not gate the artifact on research completing",
		"Pass `platform:\"mobile\"` for mobile targets, `platform:\"desktop\"` for desktop targets",
		"Call `visualise_show_widget` with a meaningful snake_case title",
		"meaningful snake_case title",
		"Text-heavy document, code reference, or reusable prose",
		"Stacking millions into bars",
		"When to visualise:",
		"prefer an inline artifact over plain text",
		"Never pass raw JSX, TSX, or an unbundled app to the output tool",
		"`visualise_read_me` is the visual styling authority",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("artifact policy missing %q:\n%s", want, prompt)
		}
	}
	for _, reject := range []string{
		"`visualize:",
		"visualize_",
		"`create_file`",
		"file-artifact tool",
		"coding-agent surface provides",
		"Claude-compatible",
	} {
		if strings.Contains(prompt, reject) {
			t.Fatalf("artifact policy must use direct Jaz visualise tools only; found %q:\n%s", reject, prompt)
		}
	}
	for _, reject := range []string{"visualise:show_widget", "visualise:read_me"} {
		if strings.Contains(prompt, reject) {
			t.Fatalf("artifact policy must use identifier-safe tool names; found %q:\n%s", reject, prompt)
		}
	}
}

func TestRenderMemoryStates(t *testing.T) {
	disabled, err := Render(testData("agents", "(empty)"))
	if err != nil {
		t.Fatal(err)
	}
	disabled = normalizeNewlines(disabled)
	if strings.Contains(disabled, "## memory") || strings.Contains(disabled, "Capture as you go") {
		t.Fatalf("nil memory must omit the memory block:\n%s", disabled)
	}
	if !strings.Contains(disabled, "## AGENTS.md") || !strings.Contains(disabled, "## SOUL.md") {
		t.Fatalf("prompt files must render regardless of memory:\n%s", disabled)
	}

	freshData := testData("(empty)", "(empty)")
	freshData.Memory = &MemoryData{LongTerm: "(empty)", ShortTerm: "(empty)"}
	fresh, err := Render(freshData)
	if err != nil {
		t.Fatal(err)
	}
	fresh = normalizeNewlines(fresh)
	assertOrder(t, fresh, "Capture as you go", "## memory/LONG_TERM.md\n\n(empty)", "## memory/SHORT_TERM.md\n\n(empty)")
	if strings.Contains(fresh, "## memory/daily/") {
		t.Fatalf("no daily content means no daily sections:\n%s", fresh)
	}
}

func TestRenderWidgetSurfaceKeepsSharedVisualPolicy(t *testing.T) {
	data := testData("agents", "soul")
	data.ArtifactSurface = "widget"
	prompt, err := Render(data)
	if err != nil {
		t.Fatal(err)
	}
	prompt = normalizeNewlines(prompt)
	for _, want := range []string{
		"## AGENTS.md\n\nagents",
		"## SOUL.md\n\nsoul",
		"## Agent delegation",
		"agent_spawn",
		"## Artifacts and visualisation",
		"Always call `visualise_read_me` before the first artifact",
		"Finish with the output contract for the current surface",
		"Launching background work is not delivery",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("widget surface missing %q:\n%s", want, prompt)
		}
	}
	for _, reject := range []string{
		"visualise_show_widget",
		"visualise_publish_widget",
		"## Board Widget Runtime",
		"Few-shot trace:",
	} {
		if strings.Contains(prompt, reject) {
			t.Fatalf("widget surface base prompt must leave output details to the loop extension; found %q:\n%s", reject, prompt)
		}
	}
}

func testData(agents, soul string) Data {
	return Data{
		Agents:   agents,
		Date:     "June 16, 2026",
		Time:     "12:34:56 BST",
		Timezone: "BST (UTC+01:00)",
		Weekday:  "Tuesday",
		Human:    "Tuesday, June 16, 2026 at 12:34:56 BST",
		Cwd:      "/tmp/jaz/workspaces/default",
		Soul:     soul,
	}
}

func assertOrder(t *testing.T, value string, parts ...string) {
	t.Helper()
	value = normalizeNewlines(value)
	offset := 0
	for _, part := range parts {
		i := strings.Index(value[offset:], part)
		if i < 0 {
			t.Fatalf("missing %q in:\n%s", part, value)
		}
		offset += i + len(part)
	}
}

func normalizeNewlines(value string) string {
	return strings.ReplaceAll(value, "\r\n", "\n")
}
