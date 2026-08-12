package acp

import (
	"encoding/json"
	"strings"
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

	plan := parseSessionConfigOptions(json.RawMessage(`{"configOptions":[{
		"id":"collaboration_mode",
		"options":[{"value":"default"},{"value":"plan"}]
	}]}`))
	if plan.planConfigID != sessionConfigCollaborationMode {
		t.Fatalf("plan config id = %q", plan.planConfigID)
	}
	unsupportedPlan := parseSessionConfigOptions(json.RawMessage(`{"configOptions":[{
		"id":"collaboration_mode",
		"options":[{"value":"plan"}]
	}]}`))
	if unsupportedPlan.planConfigID != "" {
		t.Fatalf("incomplete plan config id = %q", unsupportedPlan.planConfigID)
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
	for _, agent := range []string{"", "custom", AgentKimi, AgentGrok, AgentAntigravity} {
		if got := DefaultAgentReasoningEffort(agent); got != "" {
			t.Fatalf("%q default effort = %q, want empty", agent, got)
		}
	}
}

// Codex repeats the session's current model in the model config option even when
// the account cannot use it, so a thread pinned to an unavailable model would
// otherwise look advertised. Codex also spells one advertised id per reasoning
// effort while accepting the bare id.
func TestModelStateIgnoresEchoedCurrentModel(t *testing.T) {
	state := parseSessionModelState(json.RawMessage(`{
		"models":{"availableModels":[{"modelId":"gpt-5.6-terra[low]"},{"modelId":"gpt-5.6-terra[high]"},{"modelId":"gpt-5.5[high]"}]},
		"configOptions":[{"id":"model","category":"model","currentValue":"gpt-5.6-sol","options":[{"value":"gpt-5.6-sol"},{"value":"gpt-5.6-terra"},{"value":"gpt-5.5"}]}]
	}`))
	if _, ok := state.matchAdvertised("gpt-5.6-sol"); ok {
		t.Fatal("the echoed current model must not count as advertised")
	}
	if _, ok := state.matchAdvertised("gpt-5.6-terra"); !ok {
		t.Fatal("advertised model was not matched through its per-effort ids")
	}
	if got := strings.Join(state.advertisedModels(), ","); got != "gpt-5.5,gpt-5.6-terra" {
		t.Fatalf("advertised models = %q", got)
	}
}

// An agent that describes its models only through the config option is still
// taken at its word.
func TestModelStateFallsBackToConfigOptions(t *testing.T) {
	state := parseSessionModelState(json.RawMessage(`{
		"configOptions":[{"id":"model","category":"model","options":[{"value":"fake-large"}]}]
	}`))
	if _, ok := state.matchAdvertised("fake-large"); !ok {
		t.Fatal("config option model was not matched")
	}
	if _, ok := state.matchAdvertised("fake-small"); ok {
		t.Fatal("unlisted model was matched")
	}
}

// Codex model access follows the account's plan, so an unavailable model keeps
// the agent's default; only agents with stable model ids reject.
func TestUnadvertisedModelPolicyPerAgent(t *testing.T) {
	state := parseSessionModelState(json.RawMessage(
		`{"models":{"availableModels":[{"modelId":"gpt-5.6-terra[low]"}]}}`))
	codex := agentPolicyForAgent(AgentCodex)

	if model, err := codex.sessionModelToSend(AgentCodex, "gpt-5.6-sol", state); err != nil || model != "" {
		t.Fatalf("unavailable codex model = %q, err %v; want the agent default", model, err)
	}
	// Codex accepts the bare id and would reject the per-effort spelling, so an
	// available model must be sent exactly as configured.
	if model, err := codex.sessionModelToSend(AgentCodex, "gpt-5.6-terra", state); err != nil || model != "gpt-5.6-terra" {
		t.Fatalf("available codex model = %q, err %v", model, err)
	}
	if _, err := agentPolicyForAgent(AgentClaude).sessionModelToSend(AgentClaude, "claude-ghost-9", state); err == nil {
		t.Fatal("claude must reject an unadvertised model")
	}
	// Agents with no policy leave the id to the agent to accept or refuse.
	if model, err := agentPolicyForAgent(AgentKimi).sessionModelToSend(AgentKimi, "kimi-k3", state); err != nil || model != "kimi-k3" {
		t.Fatalf("unpoliced model = %q, err %v", model, err)
	}
}

func TestClaudeSendsAdvertisedContextTagSpelling(t *testing.T) {
	// Claude Code alternates Fable's [1m] context tag between restarts; whichever
	// spelling the catalog carries, the sent value must be the advertised one.
	bare := parseSessionModelState(json.RawMessage(
		`{"models":{"availableModels":[{"modelId":"claude-fable-5"},{"modelId":"default"},{"modelId":"opus[1m]"},{"modelId":"sonnet"}]}}`))
	tagged := parseSessionModelState(json.RawMessage(
		`{"models":{"availableModels":[{"modelId":"claude-fable-5[1m]"},{"modelId":"default"},{"modelId":"opus[1m]"},{"modelId":"sonnet"}]}}`))
	policy := agentPolicyForAgent(AgentClaude)

	for _, test := range []struct {
		name       string
		state      sessionModelState
		configured string
		want       string
	}{
		{"tagged config against bare advertisement", bare, "claude-fable-5[1m]", "claude-fable-5"},
		{"bare config against tagged advertisement", tagged, "claude-fable-5", "claude-fable-5[1m]"},
		{"exact match is unchanged", tagged, "claude-fable-5[1m]", "claude-fable-5[1m]"},
	} {
		got, err := policy.sessionModelToSend(AgentClaude, test.configured, test.state)
		if err != nil || got != test.want {
			t.Fatalf("%s = %q (err %v), want %q", test.name, got, err, test.want)
		}
	}

	// A genuinely unknown model must still be rejected.
	if _, err := policy.sessionModelToSend(AgentClaude, "claude-ghost-9", bare); err == nil {
		t.Fatal("expected unknown model to fail")
	}
}
