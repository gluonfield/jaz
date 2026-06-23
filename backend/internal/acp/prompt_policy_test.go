package acp

import (
	"reflect"
	"testing"
)

func TestRestrictedCodexWorkersDoNotDeferMCPTools(t *testing.T) {
	cfg := AgentConfig{ManagedAdapterArgs: []string{
		"-c", `sandbox_mode="danger-full-access"`,
		"-c", `features.tool_search_always_defer_mcp_tools=true`,
		"-c", `suppress_unstable_features_warning=true`,
	}}
	got := configForMCPServerPolicy(AgentCodex, cfg, MCPServerPolicyBrowserWorker)
	want := []string{
		"-c", `sandbox_mode="danger-full-access"`,
		"-c", `suppress_unstable_features_warning=true`,
	}
	if !reflect.DeepEqual(got.ManagedAdapterArgs, want) {
		t.Fatalf("managed args = %#v, want %#v", got.ManagedAdapterArgs, want)
	}
}

func TestOrdinaryCodexSessionsKeepDeferredMCPTools(t *testing.T) {
	cfg := AgentConfig{ManagedAdapterArgs: []string{
		"-c", `features.tool_search_always_defer_mcp_tools=true`,
	}}
	got := configForMCPServerPolicy(AgentCodex, cfg, MCPServerPolicyAll)
	if !reflect.DeepEqual(got.ManagedAdapterArgs, cfg.ManagedAdapterArgs) {
		t.Fatalf("managed args = %#v, want %#v", got.ManagedAdapterArgs, cfg.ManagedAdapterArgs)
	}
}
