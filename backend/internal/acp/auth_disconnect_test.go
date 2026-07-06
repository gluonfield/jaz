package acp

import "testing"

func TestDisconnectedAuthConfigPinsProfileAgents(t *testing.T) {
	for _, agent := range []string{AgentCodex, AgentClaude, AgentOpenCode, AgentAntigravity} {
		if got := DisconnectedAuthConfig(agent, AgentAuthConfig{Mode: AuthModeExistingCLI}); got.Mode != AuthModeJazProfile || got.Path != "" {
			t.Fatalf("%s disconnected auth = %#v, want Jaz profile", agent, got)
		}
	}
}

func TestNormalizeAgentAuthAllowsAntigravityExistingCLI(t *testing.T) {
	got, err := NormalizeAgentAuthConfig(AgentAntigravity, AgentAuthConfig{Mode: AuthModeExistingCLI})
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != AuthModeExistingCLI {
		t.Fatalf("mode = %q, want existing CLI", got.Mode)
	}
}

func TestDisconnectedAuthConfigKeepsGrokExistingCLI(t *testing.T) {
	got := DisconnectedAuthConfig(AgentGrok, AgentAuthConfig{Mode: AuthModeExistingCLI})
	if got.Mode != AuthModeExistingCLI || got.Path != "" {
		t.Fatalf("grok disconnected auth = %#v, want existing CLI", got)
	}
}
