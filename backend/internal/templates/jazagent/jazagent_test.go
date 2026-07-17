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
		"Use the external ACP session tools only when the user explicitly asks to run work in a separate ACP session: acp_session_create, acp_agent_options, acp_session_send, acp_session_wait, acp_session_status, acp_session_cancel, and acp_session_list.",
		"Merely discussing or reviewing an ACP harness does not authorize a separate session.",
		"Generic requests to use subagents, parallelize, or spawn child agents stay with the active agent's native collaboration tools.",
		"Do not inspect or invoke local agent CLIs unless the user explicitly asks for the local CLI.",
		"acp_session_create only creates an external session; send work with acp_session_send.",
		"Omit model overrides unless the user asks for a specific model. Use acp_agent_options({}) when you need available harnesses and useful model choices",
		"Use worktree=true for isolated repo changes; add branch when the new worktree should start from a specific branch/ref.",
		"For reviewing another session's worktree, pass that worktree as directory without worktree=true.",
		"Use plan=true for delegated planning/review/proposal tasks.",
		"If an ACP agent asks questions, say the questions are waiting above and stop.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"Delegate coding work to ACP agents", "Directory guide:", "runtime state", "default workspace", "Date:", "Time:", "Timezone:", "Weekday:", "Current working directory:", "## Jaz platform", "## memory", "## AGENTS.md", "## SOUL.md", "## INTERNAL.md", "<available_skills>"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("agent prompt must not contain %q:\n%s", forbidden, prompt)
		}
	}
}
