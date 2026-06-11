package acpcompletion

import (
	"strings"
	"testing"
)

func TestRenderFullAndMinimal(t *testing.T) {
	full, err := Render(Data{Slug: "codex-plan", Agent: "codex", State: "idle", Error: "boom", Assistant: "did the thing"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"ACP session codex-plan (codex) completed with state idle.",
		"Error: boom",
		"Assistant result:\ndid the thing",
		"Report the outcome to the user now",
	} {
		if !strings.Contains(full, want) {
			t.Fatalf("missing %q in:\n%s", want, full)
		}
	}

	minimal, err := Render(Data{Slug: "s", Agent: "claude", State: "failed"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(minimal, "Error:") || strings.Contains(minimal, "Assistant result:") {
		t.Fatalf("empty fields must not render sections:\n%s", minimal)
	}
}
