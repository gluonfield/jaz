package acp

import "testing"

func TestDisconnectedAuthConfigPinsProfileAgents(t *testing.T) {
	for _, agent := range []string{AgentCodex, AgentClaude, AgentOpenCode} {
		if got := DisconnectedAuthConfig(agent, AgentAuthConfig{Mode: AuthModeExistingCLI}); got.Mode != AuthModeJazProfile || got.Path != "" {
			t.Fatalf("%s disconnected auth = %#v, want Jaz profile", agent, got)
		}
	}
}

func TestDisconnectedAuthConfigKeepsGrokExistingCLI(t *testing.T) {
	got := DisconnectedAuthConfig(AgentGrok, AgentAuthConfig{Mode: AuthModeExistingCLI})
	if got.Mode != AuthModeExistingCLI || got.Path != "" {
		t.Fatalf("grok disconnected auth = %#v, want existing CLI", got)
	}
}
