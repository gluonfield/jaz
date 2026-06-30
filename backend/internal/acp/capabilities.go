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

func initNativeGoalSupported(raw json.RawMessage) bool {
	var resp acpschema.InitializeResponse
	if json.Unmarshal(raw, &resp) != nil || resp.AgentCapabilities == nil {
		return false
	}
	return metaNativeGoal(resp.AgentCapabilities.Meta)
}

func catalogRuntimeCapabilities(agent string) *storage.RuntimeCapabilities {
	if !CatalogAgentCapabilitiesFor(agent).NativeGoal {
		return nil
	}
	return &storage.RuntimeCapabilities{NativeGoal: true}
}

func runtimeCapabilitiesFromInit(agent string, raw json.RawMessage) *storage.RuntimeCapabilities {
	if initNativeGoalSupported(raw) {
		return &storage.RuntimeCapabilities{NativeGoal: true}
	}
	return catalogRuntimeCapabilities(agent)
}

func EffectiveRuntimeCapabilities(agent string, caps *storage.RuntimeCapabilities) *storage.RuntimeCapabilities {
	caps = storage.NormalizeRuntimeCapabilities(caps)
	catalog := catalogRuntimeCapabilities(agent)
	if caps == nil {
		return catalog
	}
	if catalog != nil && catalog.NativeGoal {
		caps.NativeGoal = true
		caps.NativeGoalNegotiable = false
	}
	return caps
}

func effectiveRuntimeNativeGoal(agent string, caps *storage.RuntimeCapabilities) bool {
	caps = EffectiveRuntimeCapabilities(agent, caps)
	return caps != nil && caps.NativeGoal
}

func storedRuntimeCapabilitiesEqual(a, b *storage.RuntimeCapabilities) bool {
	a = storage.NormalizeRuntimeCapabilities(a)
	b = storage.NormalizeRuntimeCapabilities(b)
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.NativeGoal == b.NativeGoal && a.NativeGoalNegotiable == b.NativeGoalNegotiable
}

func metaPromptQueueing(meta map[string]any) bool {
	if boolMeta(meta, "promptQueueing") {
		return true
	}
	claudeCode, _ := meta["claudeCode"].(map[string]any)
	return boolMeta(claudeCode, "promptQueueing")
}

func metaNativeGoal(meta map[string]any) bool {
	codex, _ := meta[codexMetaKey].(map[string]any)
	return boolMeta(codex, "nativeGoal")
}

func boolMeta(meta map[string]any, key string) bool {
	if v, ok := meta[key].(bool); ok && v {
		return true
	}
	return false
}
