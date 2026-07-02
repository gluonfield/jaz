package acp

import (
	"encoding/json"

	acpschema "github.com/gluonfield/acp-transport/acp"
)

func promptQueueingSupported(raw json.RawMessage) bool {
	var resp acpschema.InitializeResponse
	if json.Unmarshal(raw, &resp) != nil || resp.AgentCapabilities == nil {
		return false
	}
	return metaPromptQueueing(resp.AgentCapabilities.Meta)
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
