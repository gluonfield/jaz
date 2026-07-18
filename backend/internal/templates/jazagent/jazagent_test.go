package jazagent

import (
	"strings"
	"testing"
)

func TestRenderRulesOnly(t *testing.T) {
	prompt, err := Render()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"You are Jaz",
		"Use the Jaz agent tools only when the user explicitly asks to run work in a separate Jaz agent session: jazagent_spawn, jazagent_options, jazagent_send, jazagent_wait, jazagent_status, jazagent_cancel, and jazagent_list.",
		"Merely discussing or reviewing an agent harness does not authorize a separate session.",
		"Generic requests to use subagents, parallelize, or spawn child agents stay with the active agent's native collaboration tools.",
		"Do not inspect or invoke local agent CLIs unless the user explicitly asks for the local CLI.",
		"jazagent_spawn only creates a Jaz agent session; send work with jazagent_send.",
		"Omit model overrides unless the user asks for a specific model. Use jazagent_options({}) when you need available agents and useful model choices",
		"Use worktree=true for isolated repo changes; add branch when the new worktree should start from a specific branch/ref.",
		"For reviewing another session's worktree, pass that worktree as directory without worktree=true.",
		"Use plan=true for delegated planning/review/proposal tasks.",
		"If an ACP agent asks questions, say the questions are waiting above and stop.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"Delegate coding work to ACP agents", "acp_session_", "acp_agent_options", "Directory guide:", "runtime state", "default workspace", "Date:", "Time:", "Timezone:", "Weekday:", "Current working directory:", "## Jaz platform", "## memory", "## AGENTS.md", "## SOUL.md", "## INTERNAL.md", "<available_skills>"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("agent prompt must not contain %q:\n%s", forbidden, prompt)
		}
	}
}
