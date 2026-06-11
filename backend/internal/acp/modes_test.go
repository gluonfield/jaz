package acp

import (
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

func TestExecutionModeForAgentUsesAgentPolicy(t *testing.T) {
	modes := []acpschema.SessionMode{
		{ID: "auto", Name: "Auto"},
		{ID: "bypassPermissions", Name: "Bypass Permissions"},
		{ID: "acceptEdits", Name: "Accept Edits"},
		{ID: "plan", Name: "Plan"},
	}
	if got := executionModeForAgent(AgentClaude, modes); got != "bypassPermissions" {
		t.Fatalf("claude execution mode = %q, want bypassPermissions", got)
	}
	if got := executionModeForAgent("other", modes); got != "" {
		t.Fatalf("generic execution mode = %q, want empty", got)
	}
}

func TestModeStateFromACPDoesNotApplyExecutionPolicy(t *testing.T) {
	state := modeStateFromACP(&acpschema.SessionModeState{
		CurrentModeID: "auto",
		AvailableModes: []acpschema.SessionMode{
			{ID: "auto", Name: "Auto"},
			{ID: "bypassPermissions", Name: "Bypass Permissions"},
			{ID: "plan", Name: "Plan"},
		},
	})
	if state.ExecutionModeID != "" {
		t.Fatalf("ExecutionModeID = %q, want empty", state.ExecutionModeID)
	}
}

func TestModeStateDetectsPlanWithoutFullAccess(t *testing.T) {
	state := modeStateFromACP(&acpschema.SessionModeState{
		CurrentModeID: "default",
		AvailableModes: []acpschema.SessionMode{
			{ID: "default", Name: "Default"},
			{ID: "plan", Name: "Plan"},
		},
	})
	if state.PlanModeID != "plan" {
		t.Fatalf("PlanModeID = %q", state.PlanModeID)
	}
	if state.ExecutionModeID != "" {
		t.Fatalf("ExecutionModeID = %q, want empty", state.ExecutionModeID)
	}
}
