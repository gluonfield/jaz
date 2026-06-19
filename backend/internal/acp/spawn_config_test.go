package acp

import "testing"

func TestSpawnConfigDefaultsWidgetSurfaceToWidgetMCPPolicy(t *testing.T) {
	manager := &Manager{agents: AgentCatalog{
		"fake": AgentConfig{Command: "fake"},
	}}
	req, _, _, err := manager.spawnConfig(SpawnRequest{
		ACPAgent:               "fake",
		ArtifactSurface:        " widget ",
		SystemPromptExtensions: []string{" run context ", ""},
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
	if len(req.SystemPromptExtensions) != 1 || req.SystemPromptExtensions[0] != "run context" {
		t.Fatalf("system prompt extensions = %#v", req.SystemPromptExtensions)
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
