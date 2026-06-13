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

	withoutEffort := parseSessionConfigOptions(json.RawMessage(`{"configOptions":[{"id":"model","options":[{"value":"spark"}]}]}`))
	if !withoutEffort.configOptionsPresent || withoutEffort.effortConfigPresent {
		t.Fatalf("expected config options without effort, got %#v", withoutEffort)
	}
}

func TestClampReasoningEffort(t *testing.T) {
	sonnet := []string{"default", "low", "medium", "high", "max"}
	cases := []struct {
		effort     string
		advertised []string
		want       string
	}{
		{"xhigh", sonnet, "high"},                          // nearest weaker level
		{"high", sonnet, "high"},                           // advertised as-is
		{"xhigh", nil, "xhigh"},                            // unknown advertisement: untouched
		{"", sonnet, ""},                                   // nothing configured
		{"low", []string{"medium"}, "medium"},              // nothing weaker: nearest stronger
		{"medium", []string{"default"}, ""},                // no compatible ladder level
		{"xhigh", []string{"Default", " XHIGH "}, "xhigh"}, // case/space insensitive
	}
	for _, tc := range cases {
		if got := clampReasoningEffort(tc.effort, tc.advertised); got != tc.want {
			t.Fatalf("clampReasoningEffort(%q, %v) = %q, want %q", tc.effort, tc.advertised, got, tc.want)
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
		{AgentCodex, "max", "", true},
		{AgentGrok, "ultracode", "", true},
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
