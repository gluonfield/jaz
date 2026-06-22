package acp

import (
	"encoding/json"
	"strings"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	jazMetaKey              = "jaz"
	providerSubagentMetaKey = "providerSubagent"
)

type providerSubagentHint struct {
	summary string
	status  string
}

type providerSubagentUpdate struct {
	subagent *sessionevents.ProviderSubagentEvent
	consume  bool
}

func providerSubagentFromUpdate(agent string, update acpschema.DecodedSessionUpdate) providerSubagentUpdate {
	switch event := update.(type) {
	case acpschema.SessionInfoSessionUpdate:
		return providerSubagentUpdate{subagent: providerSubagentFromJazMeta(agent, event.Meta, providerSubagentHint{}), consume: true}
	case acpschema.ToolCallSessionUpdate:
		hint := providerSubagentHint{
			summary: firstNonEmpty(event.Title, string(event.ToolCallID)),
			status:  "running",
		}
		if subagent := providerSubagentFromJazMeta(agent, event.Meta, hint); subagent != nil {
			return providerSubagentUpdate{subagent: subagent}
		}
		return providerSubagentUpdate{subagent: claudeProviderSubagentFromMeta(agent, event.Meta, hint), consume: true}
	case acpschema.ToolCallUpdateSessionUpdate:
		hint := providerSubagentHint{
			summary: firstNonEmpty(event.Title, string(event.ToolCallID)),
			status:  "running",
		}
		if subagent := providerSubagentFromJazMeta(agent, event.Meta, hint); subagent != nil {
			return providerSubagentUpdate{subagent: subagent}
		}
		return providerSubagentUpdate{subagent: claudeProviderSubagentFromMeta(agent, event.Meta, hint), consume: true}
	case acpschema.AgentMessageChunkUpdate:
		return providerSubagentUpdate{subagent: providerSubagentFromJazMeta(agent, event.Meta, providerSubagentHint{summary: "Subagent message", status: "running"})}
	case acpschema.AgentThoughtChunkUpdate:
		return providerSubagentUpdate{subagent: providerSubagentFromJazMeta(agent, event.Meta, providerSubagentHint{summary: "Subagent thinking", status: "running"})}
	default:
		return providerSubagentUpdate{}
	}
}

func providerSubagentFromJazMeta(agent string, meta map[string]any, hint providerSubagentHint) *sessionevents.ProviderSubagentEvent {
	if meta == nil {
		return nil
	}
	if jaz, ok := mapValue(meta[jazMetaKey]); ok {
		for _, key := range []string{providerSubagentMetaKey, "provider_subagent"} {
			if raw, ok := mapValue(jaz[key]); ok {
				subagent := decodeProviderSubagent(raw)
				if subagent != nil {
					fillProviderSubagent(subagent, agent, hint)
				}
				return subagent
			}
		}
	}
	return nil
}

func claudeProviderSubagentFromMeta(agent string, meta map[string]any, hint providerSubagentHint) *sessionevents.ProviderSubagentEvent {
	if CanonicalAgentName(agent) != AgentClaude {
		return nil
	}
	claudeCode, ok := mapValue(meta["claudeCode"])
	if !ok {
		return nil
	}
	id := strings.TrimSpace(stringValue(claudeCode["parentToolUseId"]))
	if id == "" {
		return nil
	}
	return &sessionevents.ProviderSubagentEvent{
		Provider: AgentClaude,
		ID:       id,
		Status:   firstNonEmpty(hint.status, "running"),
		Summary:  hint.summary,
	}
}

func decodeProviderSubagent(raw map[string]any) *sessionevents.ProviderSubagentEvent {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var subagent sessionevents.ProviderSubagentEvent
	if err := json.Unmarshal(data, &subagent); err != nil {
		return nil
	}
	if subagent.ID == "" && subagent.ThreadID != "" {
		subagent.ID = subagent.ThreadID
	}
	if subagent.ID == "" {
		return nil
	}
	return &subagent
}

func fillProviderSubagent(subagent *sessionevents.ProviderSubagentEvent, agent string, hint providerSubagentHint) {
	if subagent.Provider == "" {
		subagent.Provider = CanonicalAgentName(agent)
	}
	if subagent.Status == "" {
		subagent.Status = hint.status
	}
	if subagent.Summary == "" {
		subagent.Summary = hint.summary
	}
}

func mapValue(value any) (map[string]any, bool) {
	out, ok := value.(map[string]any)
	return out, ok
}
