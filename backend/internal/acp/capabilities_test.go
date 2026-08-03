package acp

import (
	"encoding/json"
	"testing"
)

func TestSupportedSteerMethod(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want steerMethod
	}{
		{"prompt queue", `{"agentCapabilities":{"_meta":{"claudeCode":{"promptQueueing":true}}}}`, steerPromptQueueing},
		{"native", `{"_meta":{"steering":{"supported":true,"waitForCompletion":true}}}`, steerNative},
		{"prompt queue wins", `{"agentCapabilities":{"_meta":{"claudeCode":{"promptQueueing":true}}},"_meta":{"steering":{"supported":true,"waitForCompletion":true}}}`, steerPromptQueueing},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := supportedSteerMethod(json.RawMessage(test.raw)); got != test.want {
				t.Fatalf("supportedSteerMethod() = %q, want %q", got, test.want)
			}
		})
	}
}
