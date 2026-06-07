package send

import (
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
)

func TestPlanModeResultHidesAssistantProse(t *testing.T) {
	result := planModeResult(acp.Job{
		ID:        "session-id",
		Slug:      "codex-plan",
		ACPAgent:  "codex",
		State:     acp.StateIdle,
		Assistant: "markdown plan that should render from ACP state, not tool JSON",
		Modes: acp.ModeState{
			CurrentModeID: "plan",
			PlanModeID:    "plan",
		},
		Plan: []acp.PlanEntry{{Content: "Inspect files", Status: "completed"}},
	})

	if result["assistant"] != nil {
		t.Fatalf("assistant leaked into plan result: %#v", result)
	}
	if result["has_plan"] != true {
		t.Fatalf("has_plan = %#v, want true", result["has_plan"])
	}
}
