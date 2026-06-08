package coordinator

import (
	"strings"
	"testing"
	"time"
)

func TestRenderIncludesRuntimeContextSectionsAndSkills(t *testing.T) {
	now := time.Date(2026, 6, 2, 9, 8, 7, 0, time.FixedZone("BST", 3600))
	prompt, err := Render(now, "/tmp/jaz/workspaces/default", []Section{
		{Name: "AGENTS.md", Body: "agents"},
		{Name: "SOUL.md", Body: "soul"},
	}, "skills")
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, prompt, "Date: June 2, 2026", "Time: 09:08:07 BST", "Timezone: BST (UTC+01:00)", "Weekday: Tuesday", "Current working directory: /tmp/jaz/workspaces/default", "Directory guide:", "~/.jaz: runtime state", "~/.jaz/workspaces/default: default tool cwd", "## AGENTS.md\n\nagents", "## SOUL.md\n\nsoul", "skills")
}

func TestRenderIncludesACPInstructions(t *testing.T) {
	now := time.Date(2026, 6, 2, 9, 8, 7, 0, time.FixedZone("BST", 3600))
	prompt, err := Render(now, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"agent_spawn only starts a session; send work with agent_send.",
		"Use plan=true for delegated planning/review/proposal tasks.",
		"Send approvals without plan=true.",
		"If an ACP agent asks questions, say the questions are waiting above and stop.",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("missing %q in:\n%s", want, prompt)
		}
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
