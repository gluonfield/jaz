package acp

import (
	"encoding/json"
	"fmt"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

func promptQueueingSupported(raw json.RawMessage) bool {
	var resp acpschema.InitializeResponse
	if json.Unmarshal(raw, &resp) != nil || resp.AgentCapabilities == nil {
		return false
	}
	return metaPromptQueueing(resp.AgentCapabilities.Meta)
}

func loadSessionSupported(raw json.RawMessage) bool {
	var resp acpschema.InitializeResponse
	return json.Unmarshal(raw, &resp) == nil && resp.AgentCapabilities != nil && resp.AgentCapabilities.LoadSession
}

func validateProcessLifecycle(agent string, cfg AgentConfig, raw json.RawMessage) error {
	if turnScopedAgentProcess(cfg) && !loadSessionSupported(raw) {
		return fmt.Errorf("managed ACP agent %q requires session/load support", CanonicalAgentName(agent))
	}
	return nil
}

func sessionMaterializesOnPrompt(agent string, cfg AgentConfig) bool {
	return turnScopedAgentProcess(cfg) && CanonicalAgentName(agent) == AgentCodex
}

func metaPromptQueueing(meta map[string]any) bool {
	if boolMeta(meta, "promptQueueing") {
		return true
	}
	claudeCode, _ := meta["claudeCode"].(map[string]any)
	return boolMeta(claudeCode, "promptQueueing")
}

func boolMeta(meta map[string]any, key string) bool {
	if v, ok := meta[key].(bool); ok && v {
		return true
	}
	return false
}
