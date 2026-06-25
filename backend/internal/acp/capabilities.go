package acp

import (
	"encoding/json"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/storage"
)

func promptQueueingSupported(raw json.RawMessage) bool {
	var resp acpschema.InitializeResponse
	if json.Unmarshal(raw, &resp) != nil || resp.AgentCapabilities == nil {
		return false
	}
	return metaPromptQueueing(resp.AgentCapabilities.Meta)
}

func nativeGoalSupported(raw json.RawMessage) bool {
	var resp acpschema.InitializeResponse
	if json.Unmarshal(raw, &resp) != nil || resp.AgentCapabilities == nil {
		return false
	}
	return metaNativeGoal(resp.AgentCapabilities.Meta)
}

func runtimeCapabilitiesFromInit(raw json.RawMessage) *storage.RuntimeCapabilities {
	if !nativeGoalSupported(raw) {
		return nil
	}
	return &storage.RuntimeCapabilities{NativeGoal: true}
}

func runtimeCapabilitiesNativeGoal(caps *storage.RuntimeCapabilities) bool {
	return caps != nil && caps.NativeGoal
}

func metaPromptQueueing(meta map[string]any) bool {
	if boolMeta(meta, "promptQueueing") {
		return true
	}
	claudeCode, _ := meta["claudeCode"].(map[string]any)
	return boolMeta(claudeCode, "promptQueueing")
}

func metaNativeGoal(meta map[string]any) bool {
	jaz, _ := meta["jaz"].(map[string]any)
	return boolMeta(jaz, "nativeGoal")
}

func boolMeta(meta map[string]any, key string) bool {
	if v, ok := meta[key].(bool); ok && v {
		return true
	}
	return false
}
