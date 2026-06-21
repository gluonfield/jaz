package jazagent

import (
	"strings"
	"testing"
)

func TestRenderRulesOnly(t *testing.T) {
	prompt, err := Render("C:\\Users\\augustinas\\.jaz", "C:\\Users\\augustinas\\.jaz\\workspaces\\default")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"You are Jaz",
		"Directory guide:",
		"C:\\Users\\augustinas\\.jaz: runtime state",
		"C:\\Users\\augustinas\\.jaz\\workspaces\\default: default tool cwd.",
		"Treat user phrasing like \"spawn a codex agent\", \"launch claude\", \"delegate this\", or \"ask opencode\" as a request to use the internal ACP agent tools: agent_spawn, agent_send, agent_wait, agent_status, agent_cancel, and agent_list.",
		"Do not inspect or invoke local agent CLIs unless the user explicitly asks for the local CLI.",
		"agent_spawn only starts a session; send work with agent_send.",
		"Use worktree=true for isolated repo changes; add branch when the new worktree should start from a specific branch/ref.",
		"For reviewing another session's worktree, pass that worktree as directory without worktree=true.",
		"Use plan=true for delegated planning/review/proposal tasks.",
		"If an ACP agent asks questions, say the questions are waiting above and stop.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"Date:", "Time:", "Timezone:", "Weekday:", "Current working directory:", "## Jaz platform", "## memory", "## AGENTS.md", "## SOUL.md", "<available_skills>"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("agent prompt must not contain platform content %q:\n%s", forbidden, prompt)
		}
	}
}
