package memorydream

import (
	"strings"
	"testing"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
)

func TestAgentPromptIncludesLongTermPromotionBar(t *testing.T) {
	prompt, err := agentPrompt(jazmem.DreamRequest{
		Root: "/tmp/memory",
		Date: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC),
	}, "dreams/runs/test", "dreams/review/test")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"LONG_TERM.md is profile memory",
		"routine coding style",
		"feature decisions",
		"weak one-off contacts",
		"SHORT_TERM.md is the active working set",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("agent prompt missing %q:\n%s", want, prompt)
		}
	}
}
