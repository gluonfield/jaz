package loops

import (
	"testing"
	"time"
)

func TestNormalizeCreateDerivesNameWithoutMentionMarkup(t *testing.T) {
	input := CreateLoop{
		Prompt:   "Run [$code-review](/skills/code-review/SKILL.md) on [@backend/internal](</repos/my app/backend/internal>) daily",
		Schedule: Schedule{Kind: ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"},
	}
	normalized, _, err := NormalizeCreate(input, time.Date(2026, 6, 11, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("NormalizeCreate: %v", err)
	}
	if got, want := normalized.Name, "Run $code-review on @backend/internal daily"; got != want {
		t.Fatalf("derived name = %q, want %q", got, want)
	}
}
