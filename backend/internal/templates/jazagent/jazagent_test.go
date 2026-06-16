package jazagent

import (
	"strings"
	"testing"
	"time"
)

func TestRenderContextAndRulesOnly(t *testing.T) {
	now := time.Date(2026, 6, 2, 9, 8, 7, 0, time.FixedZone("BST", 3600))
	prompt, err := Render(now, "/tmp/jaz/workspaces/default")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"You are Jaz",
		"Date: June 2, 2026",
		"Time: 09:08:07 BST",
		"Timezone: BST (UTC+01:00)",
		"Weekday: Tuesday",
		"Current working directory: /tmp/jaz/workspaces/default",
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
	for _, forbidden := range []string{"## Jaz platform", "## memory", "## AGENTS.md", "## SOUL.md", "<available_skills>"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("agent prompt must not contain platform content %q:\n%s", forbidden, prompt)
		}
	}
}
