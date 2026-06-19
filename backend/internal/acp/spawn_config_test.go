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

func TestSpawnConfigReasoningEffortNoneAndDefault(t *testing.T) {
	agents := []string{AgentJaz, AgentCodex, AgentClaude, AgentGrok, AgentOpenCode}
	catalog := AgentCatalog{}
	for _, agent := range agents {
		catalog[agent] = AgentConfig{Command: agent, Model: "gpt-5/high", ReasoningEffort: "high"}
	}
	manager := &Manager{agents: catalog}

	for _, agent := range agents {
		t.Run(agent, func(t *testing.T) {
			_, cfg, effort, err := manager.spawnConfig(SpawnRequest{ACPAgent: agent, ReasoningEffort: "none"})
			if err != nil {
				t.Fatal(err)
			}
			wantModel := "gpt-5/high"
			if agent == AgentCodex {
				wantModel = "gpt-5"
			}
			if cfg.Model != wantModel {
				t.Fatalf("model = %q, want %q", cfg.Model, wantModel)
			}
			if effort != "" || cfg.ReasoningEffort != "" {
				t.Fatalf("effort = %q, cfg effort = %q; want no reasoning effort", effort, cfg.ReasoningEffort)
			}
			_, cfg, effort, err = manager.spawnConfig(SpawnRequest{ACPAgent: agent})
			if err != nil {
				t.Fatal(err)
			}
			if cfg.Model != "gpt-5/high" {
				t.Fatalf("model = %q, want configured default model", cfg.Model)
			}
			if effort != "high" || cfg.ReasoningEffort != "high" {
				t.Fatalf("effort = %q, cfg effort = %q; want configured default high", effort, cfg.ReasoningEffort)
			}
		})
	}
}
