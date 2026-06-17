package acp

import (
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

func TestPreferredBaselineModeIDUsesAgentPolicy(t *testing.T) {
	modes := []acpschema.SessionMode{
		{ID: "auto", Name: "Auto"},
		{ID: "bypassPermissions", Name: "Bypass Permissions"},
		{ID: "acceptEdits", Name: "Accept Edits"},
		{ID: "plan", Name: "Plan"},
	}
	if got := preferredBaselineModeID(AgentClaude, modes); got != "bypassPermissions" {
		t.Fatalf("claude baseline mode = %q, want bypassPermissions", got)
	}
	if got := preferredBaselineModeID("other", modes); got != "" {
		t.Fatalf("generic baseline mode = %q, want empty", got)
	}
}

func TestPreferredBaselineModeIDFallsBackToClaudeAuto(t *testing.T) {
	modes := []acpschema.SessionMode{
		{ID: "auto", Name: "Auto"},
		{ID: "acceptEdits", Name: "Accept Edits"},
		{ID: "plan", Name: "Plan"},
	}
	if got := preferredBaselineModeID(AgentClaude, modes); got != "auto" {
		t.Fatalf("claude baseline mode = %q, want auto fallback", got)
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
	if state.CurrentModeID != "auto" || state.PlanModeID != "plan" {
		t.Fatalf("state = %#v", state)
	}
}

func TestBaselineModeIDAvoidsPlanMode(t *testing.T) {
	modes := ModeState{
		CurrentModeID: "plan",
		PlanModeID:    "plan",
		AvailableModes: []ModeSnapshot{
			{ID: "always-approve", Name: "Always Approve"},
			{ID: "plan", Name: "Plan"},
		},
	}
	if got := baselineModeID(AgentGrok, modes); got != "always-approve" {
		t.Fatalf("baseline mode = %q, want always-approve", got)
	}
	modes.CurrentModeID = "manual"
	modes.AvailableModes = []ModeSnapshot{
		{ID: "manual", Name: "Manual"},
		{ID: "plan", Name: "Plan"},
	}
	if got := baselineModeID("other", modes); got != "manual" {
		t.Fatalf("baseline mode = %q, want manual", got)
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
}
