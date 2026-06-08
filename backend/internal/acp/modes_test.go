package acp

import (
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

func TestPreferredExecutionModeAcceptsAgentFlavors(t *testing.T) {
	for _, id := range fullAccessModes {
		got := preferredExecutionMode([]acpschema.SessionMode{{ID: acpschema.SessionModeID(id)}})
		if got != id {
			t.Fatalf("preferredExecutionMode(%q) = %q", id, got)
		}
	}
}

func TestModeStateDoesNotForceClaudeBypassPermissions(t *testing.T) {
	state := modeStateFromACP(&acpschema.SessionModeState{
		CurrentModeID: "auto",
		AvailableModes: []acpschema.SessionMode{
			{ID: "auto", Name: "Auto"},
			{ID: "bypassPermissions", Name: "Bypass Permissions"},
			{ID: "plan", Name: "Plan"},
		},
	})
	if state.ExecutionModeID != "auto" {
		t.Fatalf("ExecutionModeID = %q, want auto", state.ExecutionModeID)
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
	if state.ExecutionModeID != "default" {
		t.Fatalf("ExecutionModeID = %q", state.ExecutionModeID)
	}
}
