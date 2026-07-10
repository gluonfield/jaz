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
	got := argsForLaunchPolicy(AgentCodex, input, MCPServerPolicyBrowserWorker)
	want := []string{
		"-c", `sandbox_mode="danger-full-access"`,
		"-c", `suppress_unstable_features_warning=true`,
		"-c", `features.goals=false`,
		"-c", `features.browser_use=false`,
		"-c", `features.browser_use_external=false`,
		"-c", `features.in_app_browser=false`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}

func TestCodexSessionsDisableNativeGoals(t *testing.T) {
	input := []string{
		"-c", `features.tool_search_always_defer_mcp_tools=true`,
		"-c", `features.goals=true`,
		`-c=features.goals=true`,
		"--", "payload",
	}
	got := argsForLaunchPolicy(AgentCodex, input, MCPServerPolicyAll)
	want := []string{
		"-c", `features.tool_search_always_defer_mcp_tools=true`,
		"-c", `features.goals=false`,
		"--", "payload",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}
