package acp

import (
	"encoding/json"
	"testing"
)

// Shape returned by claude-agent-acp@0.44.0 for session/set_config_option
// after switching to sonnet: xhigh is gone from the effort levels.
const sonnetConfigResponse = `{
	"configOptions": [
		{"id": "mode", "type": "select", "currentValue": "auto", "options": [{"value": "auto"}]},
		{"id": "model", "type": "select", "currentValue": "sonnet", "options": [{"value": "default"}, {"value": "sonnet"}]},
		{"id": "effort", "type": "select", "currentValue": "default", "options": [
			{"value": "default"}, {"value": "low"}, {"value": "medium"}, {"value": "high"}, {"value": "max"}
		]}
	]
}`

func TestParseEffortOptions(t *testing.T) {
	options := parseEffortOptions(json.RawMessage(sonnetConfigResponse))
	if len(options) != 5 || options[0] != "default" || options[4] != "max" {
		t.Fatalf("options = %#v", options)
	}
	if parseEffortOptions(nil) != nil {
		t.Fatal("expected nil for empty raw")
	}
	if parseEffortOptions(json.RawMessage(`{"configOptions":[{"id":"mode","options":[]}]}`)) != nil {
		t.Fatal("expected nil when no effort option is advertised")
	}
}

func TestParseSessionConfigOptions(t *testing.T) {
	state := parseSessionConfigOptions(json.RawMessage(sonnetConfigResponse))
	if !state.configOptionsPresent {
		t.Fatal("expected config options to be present")
	}
	if !state.effortConfigPresent {
		t.Fatal("expected effort config to be present")
	}
	if len(state.modelOptions) != 2 || state.modelOptions[1] != "sonnet" {
		t.Fatalf("model options = %#v", state.modelOptions)
	}
	if len(state.effortOptions) != 5 || state.effortOptions[4] != "max" {
		t.Fatalf("effort options = %#v", state.effortOptions)
	}
	if state.effortConfigID != "effort" {
		t.Fatalf("effort config id = %q", state.effortConfigID)
	}

	withoutEffort := parseSessionConfigOptions(json.RawMessage(`{"configOptions":[{"id":"model","options":[{"value":"spark"}]}]}`))
	if !withoutEffort.configOptionsPresent || withoutEffort.effortConfigPresent {
		t.Fatalf("expected config options without effort, got %#v", withoutEffort)
	}

	byCategory := parseSessionConfigOptions(json.RawMessage(`{"configOptions":[
		{"id":"active_model","category":"model","options":[{"value":"spark"}]},
		{"id":"thinking","category":"thought_level","options":[{"value":"high"}]}
	]}`))
	if byCategory.effortConfigID != "thinking" || len(byCategory.effortOptions) != 1 || byCategory.effortOptions[0] != "high" {
		t.Fatalf("category effort config = %#v", byCategory)
	}
	if len(byCategory.modelOptions) != 1 || byCategory.modelOptions[0] != "spark" {
		t.Fatalf("category model options = %#v", byCategory.modelOptions)
	}

	mixed := parseSessionConfigOptions(json.RawMessage(`{"configOptions":[
		{"id":"thinking","category":"thought_level","options":[{"value":"xhigh"}]},
		{"id":"effort","options":[{"value":"high"}]}
	]}`))
	if mixed.effortConfigID != "thinking" || len(mixed.effortOptions) != 1 || mixed.effortOptions[0] != "xhigh" {
		t.Fatalf("mixed effort config = %#v", mixed)
	}
}

func TestConfigOptionValueAvailable(t *testing.T) {
	cases := []struct {
		options []string
		value   string
		want    bool
	}{
		{[]string{"default", "low", "high"}, "high", true},
		{[]string{"Default", " XHIGH "}, "xhigh", true},
		{[]string{"default", "high"}, "xhigh", false},
		{nil, "high", false},
	}
	for _, tc := range cases {
		if got := configOptionValueAvailable(tc.options, tc.value); got != tc.want {
			t.Fatalf("configOptionValueAvailable(%v, %q) = %v, want %v", tc.options, tc.value, got, tc.want)
		}
	}
}

func TestNormalizeAgentReasoningEffort(t *testing.T) {
	cases := []struct {
		agent string
		value string
		want  string
		err   bool
	}{
		{AgentClaude, "max", "max", false},
		{AgentClaude, "ultracode", "ultracode", false},
		{AgentCodex, "max", "max", false},
		{AgentCodex, "ultra", "ultra", false},
		{AgentKimi, "on", "", false},
		{AgentGrok, "ultracode", "", true},
		{AgentGrok, "ultra", "", true},
		{AgentOpenCode, "medium", "medium", false},
		{AgentOpenCode, "max", "max", false},
	}
	for _, tc := range cases {
		got, err := NormalizeAgentReasoningEffort(tc.agent, tc.value)
		if tc.err {
			if err == nil {
				t.Fatalf("NormalizeAgentReasoningEffort(%q, %q) succeeded, want error", tc.agent, tc.value)
			}
			continue
		}
		if err != nil || got != tc.want {
			t.Fatalf("NormalizeAgentReasoningEffort(%q, %q) = %q, %v; want %q", tc.agent, tc.value, got, err, tc.want)
		}
	}
}

func TestDefaultAgentReasoningEffort(t *testing.T) {
	for _, agent := range []string{AgentCodex, AgentClaude, AgentOpenCode} {
		if got := DefaultAgentReasoningEffort(agent); got != "xhigh" {
			t.Fatalf("%s default effort = %q, want xhigh", agent, got)
		}
	}
	for _, agent := range []string{"", "custom", AgentKimi, AgentQwen, AgentGrok, AgentAntigravity} {
		if got := DefaultAgentReasoningEffort(agent); got != "" {
			t.Fatalf("%q default effort = %q, want empty", agent, got)
		}
	}
}

func TestResolveAdvertisedContextTag(t *testing.T) {
	// Claude Code alternates Fable's [1m] context tag between restarts; whichever
	// spelling the catalog carries, resolution must land on the advertised one.
	bare := parseSessionModelState(json.RawMessage(
		`{"models":{"availableModels":[{"modelId":"claude-fable-5"},{"modelId":"default"},{"modelId":"opus[1m]"},{"modelId":"sonnet"}]}}`))
	tagged := parseSessionModelState(json.RawMessage(
		`{"models":{"availableModels":[{"modelId":"claude-fable-5[1m]"},{"modelId":"default"},{"modelId":"opus[1m]"},{"modelId":"sonnet"}]}}`))

	if got := bare.resolveAdvertised("claude-fable-5[1m]"); got != "claude-fable-5" {
		t.Fatalf("tagged config against bare advertisement = %q, want claude-fable-5", got)
	}
	if got := tagged.resolveAdvertised("claude-fable-5"); got != "claude-fable-5[1m]" {
		t.Fatalf("bare config against tagged advertisement = %q, want claude-fable-5[1m]", got)
	}
	if got := tagged.resolveAdvertised("claude-fable-5[1m]"); got != "claude-fable-5[1m]" {
		t.Fatalf("exact match should be unchanged, got %q", got)
	}

	policy := agentPolicyForAgent(AgentClaude)
	for _, st := range []sessionModelState{bare, tagged} {
		resolved := st.resolveAdvertised("claude-fable-5[1m]")
		if err := policy.validateConfiguredSessionModel(AgentClaude, "claude-fable-5[1m]", resolved, st); err != nil {
			t.Fatalf("validate after resolve: %v", err)
		}
	}

	// A genuinely unknown model must still be rejected.
	if err := policy.validateConfiguredSessionModel(AgentClaude, "claude-ghost-9", bare.resolveAdvertised("claude-ghost-9"), bare); err == nil {
		t.Fatal("expected unknown model to fail validation")
	}
}
