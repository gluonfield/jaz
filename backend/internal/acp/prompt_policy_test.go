package acp

import (
	"reflect"
	"testing"
)

func TestRestrictedCodexWorkersDoNotDeferMCPTools(t *testing.T) {
	input := []string{
		"-c", `sandbox_mode="danger-full-access"`,
		"-c", `features.tool_search_always_defer_mcp_tools=true`,
		"-c", `features.browser_use=true`,
		"-c=features.browser_use_external=true",
		"-c", `features.in_app_browser=true`,
		"-c", `suppress_unstable_features_warning=true`,
	}
	cfg := AgentConfig{Args: append([]string(nil), input...), ManagedAdapterArgs: append([]string(nil), input...)}
	got := configForMCPServerPolicy(AgentCodex, cfg, MCPServerPolicyBrowserWorker)
	want := []string{
		"-c", `sandbox_mode="danger-full-access"`,
		"-c", `suppress_unstable_features_warning=true`,
		"-c", `features.browser_use=false`,
		"-c", `features.browser_use_external=false`,
		"-c", `features.in_app_browser=false`,
	}
	if !reflect.DeepEqual(got.ManagedAdapterArgs, want) {
		t.Fatalf("managed args = %#v, want %#v", got.ManagedAdapterArgs, want)
	}
	if !reflect.DeepEqual(got.Args, want) {
		t.Fatalf("args = %#v, want %#v", got.Args, want)
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
