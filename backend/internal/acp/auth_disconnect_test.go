package acp

import "testing"

func TestDisconnectedAuthConfigPinsProfileAgents(t *testing.T) {
	for _, agent := range []string{AgentCodex, AgentClaude, AgentOpenCode} {
		if got := DisconnectedAuthConfig(agent, AgentAuthConfig{Mode: AuthModeExistingCLI}); got.Mode != AuthModeJazProfile || got.Path != "" {
			t.Fatalf("%s disconnected auth = %#v, want Jaz profile", agent, got)
		}
	}
}

func TestDisconnectedAuthConfigResetsAntigravityToAuto(t *testing.T) {
	if got := DisconnectedAuthConfig(AgentAntigravity, AgentAuthConfig{Mode: AuthModeExistingCLI}); got.Mode != AuthModeAuto || got.Path != "" {
		t.Fatalf("antigravity disconnected auth = %#v, want auto", got)
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

func TestNormalizeAgentAuthMapsAntigravityJazProfileToAuto(t *testing.T) {
	got, err := NormalizeAgentAuthConfig(AgentAntigravity, AgentAuthConfig{Mode: AuthModeJazProfile, Path: "/tmp/profile"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != AuthModeAuto || got.Path != "" {
		t.Fatalf("auth = %#v, want auto", got)
	}
}

func TestDisconnectedAuthConfigKeepsGrokExistingCLI(t *testing.T) {
	got := DisconnectedAuthConfig(AgentGrok, AgentAuthConfig{Mode: AuthModeExistingCLI})
	if got.Mode != AuthModeExistingCLI || got.Path != "" {
		t.Fatalf("grok disconnected auth = %#v, want existing CLI", got)
	}
}
