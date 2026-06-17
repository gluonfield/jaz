package memorydream

import (
	"strings"
	"testing"
	"time"

	"github.com/gluonfield/jazmem/pkg/jazmem"
)

func TestAgentPromptIncludesLongTermPromotionBar(t *testing.T) {
	prompt := agentPrompt(jazmem.DreamRequest{
		Root: "/tmp/memory",
		Date: time.Date(2026, 6, 17, 9, 0, 0, 0, time.UTC),
	}, "dreams/runs/test", "dreams/review/test")
	for _, want := range []string{
		"LONG_TERM.md is profile-level memory",
		"routine coding style preferences",
		"feature decisions",
		"one-off meeting",
		"Short-term horizon policy",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("agent prompt missing %q:\n%s", want, prompt)
		}
	}
}
