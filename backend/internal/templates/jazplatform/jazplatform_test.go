package jazplatform

import (
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/connections"
)

// A turn's date, time and directory close the prompt, so everything before
// them stays byte-identical across turns and the provider's prompt cache holds.
func TestTurnContextClosesThePrompt(t *testing.T) {
	data := Data{Agents: "agents", Date: "June 16, 2026", Time: "12:34:56 BST", Timezone: "BST (UTC+01:00)", Weekday: "Tuesday", Human: "Tuesday, June 16, 2026 at 12:34:56 BST", Cwd: "/tmp/a", Skills: "skills-block"}
	first, err := RenderThread(data)
	if err != nil {
		t.Fatal(err)
	}
	data.Date, data.Time, data.Weekday, data.Human, data.Cwd = "June 17, 2026", "08:00:00 BST", "Wednesday", "Wednesday, June 17, 2026 at 08:00:00 BST", "/tmp/b"
	second, err := RenderThread(data)
	if err != nil {
		t.Fatal(err)
	}
	cut := strings.Index(first, "Date: June 16, 2026")
	if cut < 0 || !strings.HasPrefix(second, first[:cut]) || !strings.Contains(second[cut:], "/tmp/b") {
		t.Fatalf("the turn context changed the prompt before its end:\n%s\n---\n%s", first, second)
	}
}

func TestRenderNamesEverySurfaceExplicitly(t *testing.T) {
	prompt, err := RenderThread(Data{
		Agents:     "agents",
		AgentNames: []string{"codex", "claude"},
		Date:       "June 16, 2026",
		Time:       "12:34:56 BST",
		Timezone:   "BST (UTC+01:00)",
		Weekday:    "Tuesday",
		Human:      "Tuesday, June 16, 2026 at 12:34:56 BST",
		Cwd:        "/tmp/jaz/workspaces/default/.worktrees/task",
		RuntimePaths: RuntimePaths{
			Root:             "/tmp/jaz",
			SkillsPath:       "/tmp/jaz/skills",
			SessionsPath:     "/tmp/jaz/sessions",
			DefaultWorkspace: "/tmp/jaz/workspaces/default",
			WorktreesPath:    "/tmp/jaz/workspaces/default/.worktrees",
		},
		BrowserEnabled: true,
		Memory: &MemoryData{
			LongTerm:            "- Goal: $5m.",
			ShortTerm:           "- Focus: jaz memory.",
			ShortTermCharacters: 20,
			TodayName:           "daily/2026-06-11.md",
			Today:               "- shipped templates",
		},
		Connections: []connections.AgentConnection{{
			ProviderName: "Telegram",
			Account:      "personal (42)",
			RelevantPaths: []connections.AgentPath{{
				Path:        "sources/chat/telegram/42/contacts.md",
				Kind:        connections.AgentPathKindMemoryPage,
				Explanation: "Clean contact index.",
			}, {
				Path:        "sources/chat/telegram/42/conversations/",
				Kind:        connections.AgentPathKindMemoryPrefix,
				Explanation: "Materialized chat days.",
			}},
		}},
		Skills: "skills-block",
	})
	if err != nil {
		t.Fatal(err)
	}
	prompt = normalizeNewlines(prompt)
	assertOrder(t, prompt,
		"## Jaz platform",
		"## Structured progress",
		"Do not batch status changes at the end.",
		"## Runtime paths",
		"/tmp/jaz: runtime state",
		"/tmp/jaz/workspaces/default/.worktrees: ACP worktrees.",
		"Standing instructions (`AGENTS.md`) are stored in Jaz's database",
		"## AGENTS.md\n\nagents",
		"## connections",
		"Connected accounts and their agent-relevant memory paths",
		"Telegram: personal (42)",
		"`sources/chat/telegram/42/contacts.md` (memory_page)",
		"`sources/chat/telegram/42/conversations/` (memory_prefix)",
		"## Jaz threads",
		"one of: `codex`, `claude`",
		"## Browser tools",
		"Jaz has an integrated browser exposed through the Jaztools `browser_*` tools.",
		"Start with `browser_navigate` and `browser_read_page`",
		"Do not locate or launch Chrome/Chromium binaries",
		"unless the user explicitly requests it or an existing repository test requires it",
		"check `browser_status` and report the blocker",
		"## Artifacts and visualisation",
		"Always call `visualise_read_me` before the first artifact",
		"Deliver with `visualise_show_widget`",
		"skills-block",
		"## memory",
		"broad context from the user's past behavior",
		"start from the user's question",
		"available memory search tool",
		"Capture as you go",
		"current size is 20/5,000 characters",
		"## memory/LONG_TERM.md\n\n- Goal: $5m.",
		"## memory/SHORT_TERM.md\n\n- Focus: jaz memory.",
		"## memory/daily/2026-06-11.md\n\n- shipped templates",
	)
	if strings.Contains(prompt, "You are Jaz") {
		t.Fatalf("platform prompt must not carry the coordinator identity:\n%s", prompt)
	}
	for _, want := range []string{
		"these govern behavior on the Jaz platform",
		"Launching background work is not delivery",
		"Requests to build or change Jaz, a project, a generated document or its prompts belong in that project's files",
		"`AGENTS.md` contains the user's standing rules for durable instructions",
		"Use `create_thread` only when",
		"Merely discussing or reviewing one of these agent harnesses does not authorize a separate session",
		"Omit model overrides unless the user asks for a specific model",
		"Generic requests to use subagents, parallelize, or spawn child agents stay with the active agent's native collaboration tools",
		"a Markdown file path on its own paragraph renders as a document preview card",
		"Make one whenever a picture explains the answer better than text",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("artifact policy missing %q:\n%s", want, prompt)
		}
	}
	for _, reject := range []string{
		"## Protected provider data",
		"integration_oauth_tokens",
		"token_json",
		"Do not inspect raw provider archives",
		"the output tool",
		"`visualize:",
		"visualize_",
		"`create_file`",
		"file-artifact tool",
		"coding-agent surface provides",
		"Claude-compatible",
		"SOUL.md",
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

func TestRenderOmitsBrowserPolicyWhenDisabled(t *testing.T) {
	prompt, err := RenderThread(testData("agents"))
	if err != nil {
		t.Fatal(err)
	}
	for _, reject := range []string{"## Browser tools", "Jaz has an integrated browser", "`browser_*`", "browser or extension interaction"} {
		if strings.Contains(prompt, reject) {
			t.Fatalf("disabled browser tools must omit %q:\n%s", reject, prompt)
		}
	}
}

func TestRenderMemoryStates(t *testing.T) {
	disabled, err := RenderThread(testData("agents"))
	if err != nil {
		t.Fatal(err)
	}
	disabled = normalizeNewlines(disabled)
	if strings.Contains(disabled, "## memory") || strings.Contains(disabled, "Capture as you go") {
		t.Fatalf("nil memory must omit the memory block:\n%s", disabled)
	}
	if !strings.Contains(disabled, "## AGENTS.md") || strings.Contains(disabled, "## SOUL.md") {
		t.Fatalf("AGENTS.md must render and SOUL.md must be omitted:\n%s", disabled)
	}

	freshData := testData("(empty)")
	freshData.Memory = &MemoryData{LongTerm: "(empty)", ShortTerm: "(empty)"}
	fresh, err := RenderThread(freshData)
	if err != nil {
		t.Fatal(err)
	}
	fresh = normalizeNewlines(fresh)
	assertOrder(t, fresh, "Capture as you go", "## memory/LONG_TERM.md\n\n(empty)", "## memory/SHORT_TERM.md\n\n(empty)")
	if strings.Contains(fresh, "## memory/daily/") {
		t.Fatalf("no daily content means no daily sections:\n%s", fresh)
	}
}

func TestRenderStandaloneModules(t *testing.T) {
	connectionsPrompt, err := RenderConnections([]connections.AgentConnection{{
		ProviderName: "WhatsApp",
		Account:      "personal (+447700900123)",
		RelevantPaths: []connections.AgentPath{{
			Path:        "sources/chat/whatsapp/447700900123/contacts.md",
			Kind:        connections.AgentPathKindMemoryPage,
			Explanation: "Clean contacts.",
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## connections", "`plugins_search`", "available memory search tool", "WhatsApp: personal", "`sources/chat/whatsapp/447700900123/contacts.md` (memory_page)", "Clean contacts."} {
		if !strings.Contains(connectionsPrompt, want) {
			t.Fatalf("connections prompt missing %q:\n%s", want, connectionsPrompt)
		}
	}
	if empty, err := RenderConnections(nil); err != nil || !strings.Contains(empty, "`plugins_search`") || strings.Contains(empty, "Connected accounts") {
		t.Fatalf("empty connections = %q err=%v", empty, err)
	}

	memory, err := RenderMemory(&MemoryData{Root: "/tmp/jaz/memory", LongTerm: "- long", ShortTerm: "- short", ShortTermCharacters: 7})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"## memory", "current size is 7/5,000 characters", "## memory/LONG_TERM.md\n\n- long", "## memory/SHORT_TERM.md\n\n- short"} {
		if !strings.Contains(memory, want) {
			t.Fatalf("memory prompt missing %q:\n%s", want, memory)
		}
	}
	if empty, err := RenderMemory(nil); err != nil || empty != "" {
		t.Fatalf("empty memory = %q err=%v", empty, err)
	}
}

func TestRenderWidgetSurfaceKeepsSharedVisualPolicy(t *testing.T) {
	data := testData("agents")
	data.ArtifactSurface = "widget"
	prompt, err := RenderThread(data)
	if err != nil {
		t.Fatal(err)
	}
	prompt = normalizeNewlines(prompt)
	for _, want := range []string{
		"## AGENTS.md\n\nagents",
		"## Jaz threads",
		"create_thread",
		"list_agent_options",
		"## Artifacts and visualisation",
		"Always call `visualise_read_me` before the first artifact",
		"Deliver with `visualise_publish_widget`",
		"Launching background work is not delivery",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("widget surface missing %q:\n%s", want, prompt)
		}
	}
	for _, reject := range []string{
		"visualise_show_widget",
		"## Board Widget Runtime",
	} {
		if strings.Contains(prompt, reject) {
			t.Fatalf("widget surface base prompt must leave output details to the loop extension; found %q:\n%s", reject, prompt)
		}
	}
}

func TestRenderCloudPromptModeUsesMCPInstructions(t *testing.T) {
	data := Data{
		Agents: "User standing rule",
		Cloud: &CloudInstructions{
			HasOrg:      true,
			OrgAgents:   "Org compliance rule",
			HasSpace:    true,
			SpaceAgents: "Space tender rule",
		},
		Date:     "June 16, 2026",
		Time:     "12:34:56 BST",
		Timezone: "BST (UTC+01:00)",
		Weekday:  "Tuesday",
		Human:    "Tuesday, June 16, 2026 at 12:34:56 BST",
		Cwd:      "/workspace",
	}
	prompt, err := RenderThread(data)
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, prompt,
		"## Instructions",
		"`instruction_get`",
		"`instruction_update`",
		"`instruction_propose`",
		"## Org AGENTS.md\n\nOrg compliance rule",
		"## Space AGENTS.md\n\nSpace tender rule",
		"## AGENTS.md\n\nUser standing rule",
	)
	if strings.Contains(prompt, "## Runtime paths") || strings.Contains(prompt, "SOUL.md") {
		t.Fatalf("cloud prompt must omit local Runtime paths and SOUL.md:\n%s", prompt)
	}

	botPrompt, err := RenderBot(BotData{Data: data, Instructions: "Bot cloud rules"})
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, botPrompt,
		"## BOTS.md",
		"`instruction_get`",
		"`instruction_update`",
		"Bot cloud rules",
	)
	if strings.Contains(botPrompt, "/BOTS.md") {
		t.Fatalf("cloud bot prompt must omit local BOTS.md path:\n%s", botPrompt)
	}
}

func testData(agents string) Data {
	return Data{
		Agents:     agents,
		AgentNames: []string{"codex"},
		Date:       "June 16, 2026",
		Time:       "12:34:56 BST",
		Timezone:   "BST (UTC+01:00)",
		Weekday:    "Tuesday",
		Human:      "Tuesday, June 16, 2026 at 12:34:56 BST",
		Cwd:        "/tmp/jaz/workspaces/default",
		RuntimePaths: RuntimePaths{
			Root:             "/tmp/jaz",
			SkillsPath:       "/tmp/jaz/skills",
			SessionsPath:     "/tmp/jaz/sessions",
			DefaultWorkspace: "/tmp/jaz/workspaces/default",
			WorktreesPath:    "/tmp/jaz/workspaces/default/.worktrees",
		},
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
