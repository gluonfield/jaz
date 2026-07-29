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

func sessionRestoreMethod(raw json.RawMessage) string {
	var resp acpschema.InitializeResponse
	if json.Unmarshal(raw, &resp) != nil || resp.AgentCapabilities == nil {
		return ""
	}
	if sessions := resp.AgentCapabilities.SessionCapabilities; sessions != nil && sessions.Resume != nil {
		return acpschema.AgentMethodSessionResume
	}
	if resp.AgentCapabilities.LoadSession {
		return acpschema.AgentMethodSessionLoad
	}
	return ""
}

func validateProcessLifecycle(agent string, cfg AgentConfig, raw json.RawMessage) error {
	if turnScopedAgentProcess(cfg) && sessionRestoreMethod(raw) == "" {
		return fmt.Errorf("managed ACP agent %q requires session/resume or session/load support", CanonicalAgentName(agent))
	}
	return nil
}

func sessionMaterializesOnPrompt(agent string, cfg AgentConfig) bool {
	return turnScopedAgentProcess(cfg) && agentPolicyForAgent(agent).materializesOnPrompt
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
