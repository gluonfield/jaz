package acp

import (
	"context"
	"strings"
	"testing"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

// Agents name their unattended mode differently (Codex "full-access", Claude
// Code "bypassPermissions", Gemini CLI "yolo"); all must be accepted as-is.
func TestRequireFullAccessModeAcceptsAgentFlavors(t *testing.T) {
	for _, id := range fullAccessModes {
		session := acpschema.NewSessionResponse{Modes: &acpschema.SessionModeState{
			CurrentModeID: acpschema.SessionModeID(id),
		}}
		if err := requireFullAccessMode(context.Background(), nil, session); err != nil {
			t.Fatalf("current mode %q should be accepted: %v", id, err)
		}
	}
}

func TestRequireFullAccessModeRejectsWhenUnavailable(t *testing.T) {
	session := acpschema.NewSessionResponse{Modes: &acpschema.SessionModeState{
		CurrentModeID:  "default",
		AvailableModes: []acpschema.SessionMode{{ID: "default"}, {ID: "plan"}},
	}}
	err := requireFullAccessMode(context.Background(), nil, session)
	if err == nil {
		t.Fatal("want error when no full-access-equivalent mode is offered")
	}
	if !strings.Contains(err.Error(), "bypassPermissions") {
		t.Fatalf("error should name the accepted modes, got: %v", err)
	}
}
