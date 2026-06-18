package acp

import "testing"

func TestSpawnConfigDefaultsWidgetSurfaceToWidgetMCPPolicy(t *testing.T) {
	manager := &Manager{agents: AgentCatalog{
		"fake": AgentConfig{Command: "fake"},
	}}
	req, _, _, err := manager.spawnConfig(SpawnRequest{
		ACPAgent:        "fake",
		ArtifactSurface: " widget ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.ArtifactSurface != "widget" {
		t.Fatalf("artifact surface = %q", req.ArtifactSurface)
	}
	if req.MCPServerPolicy != MCPServerPolicyWidget {
		t.Fatalf("mcp server policy = %q, want %q", req.MCPServerPolicy, MCPServerPolicyWidget)
	}
}

func TestSpawnConfigPreservesExplicitMCPPolicy(t *testing.T) {
	manager := &Manager{agents: AgentCatalog{
		"fake": AgentConfig{Command: "fake"},
	}}
	req, _, _, err := manager.spawnConfig(SpawnRequest{
		ACPAgent:        "fake",
		ArtifactSurface: "widget",
		MCPServerPolicy: MCPServerPolicyMemorySearchWorker,
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.MCPServerPolicy != MCPServerPolicyMemorySearchWorker {
		t.Fatalf("mcp server policy = %q", req.MCPServerPolicy)
	}
}
