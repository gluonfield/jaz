package jazagent

import (
	"strings"
	"testing"
)

func TestRenderRulesOnly(t *testing.T) {
	prompt := Render()
	for _, want := range []string{
		"You are Jaz",
		"Directory guide:",
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
