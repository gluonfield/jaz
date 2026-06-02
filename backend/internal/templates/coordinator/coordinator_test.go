package coordinator

import (
	"strings"
	"testing"
	"time"
)

func TestRenderIncludesRuntimeContextSectionsAndSkills(t *testing.T) {
	now := time.Date(2026, 6, 2, 9, 8, 7, 0, time.FixedZone("BST", 3600))
	prompt, err := Render(now, []Section{
		{Name: "AGENTS.md", Content: "agents"},
		{Name: "SOUL.md", Content: "soul"},
	}, "skills")
	if err != nil {
		t.Fatal(err)
	}
	assertOrder(t, prompt, "Date: June 2, 2026", "Time: 09:08:07 BST", "Timezone: BST (UTC+01:00)", "Weekday: Tuesday", "## AGENTS.md\n\nagents", "## SOUL.md\n\nsoul", "skills")
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
