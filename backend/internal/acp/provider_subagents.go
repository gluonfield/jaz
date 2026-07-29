package acp

import (
	"encoding/json"
	"strings"

	acpschema "github.com/gluonfield/acp-transport/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

const providerSubagentsMetaKey = "providerSubagents"

type providerSubagentHint struct {
	summary string
	status  string
}

type providerSubagentUpdate struct {
	subagents []sessionevents.ProviderSubagentEvent
	consume   bool
}

// providerSubagentFromUpdate publishes subagent panel records and decides which
// updates to keep out of the main transcript.
func providerSubagentFromUpdate(agent string, update acpschema.DecodedSessionUpdate) providerSubagentUpdate {
	switch event := update.(type) {
	case acpschema.SessionInfoSessionUpdate:
		subagents := providerSubagentsFromMeta(agent, event.Meta, providerSubagentHint{})
		return providerSubagentUpdate{subagents: subagents, consume: len(subagents) > 0}
	case acpschema.ToolCallSessionUpdate:
		return toolCallSubagent(agent, event.Meta)
	case acpschema.ToolCallUpdateSessionUpdate:
		return toolCallSubagent(agent, event.Meta)
	case acpschema.AgentMessageChunkUpdate:
		return providerSubagentUpdate{subagents: providerSubagentsFromMeta(agent, event.Meta, providerSubagentHint{summary: "Subagent message", status: "running"})}
	case acpschema.AgentThoughtChunkUpdate:
		return providerSubagentUpdate{subagents: providerSubagentsFromMeta(agent, event.Meta, providerSubagentHint{summary: "Subagent thinking", status: "running"})}
	default:
		return providerSubagentUpdate{}
	}
}

func toolCallSubagent(agent string, meta map[string]any) providerSubagentUpdate {
	return providerSubagentUpdate{
		subagents: providerSubagentsFromMeta(agent, meta, providerSubagentHint{status: "running"}),
		consume:   subagentInternalToolCall(meta),
	}
}

// subagentInternalToolCall reports whether a tool call is a Claude subagent's
// own nested call (claudeCode.parentToolUseId), which Jaz keeps out of the main
// transcript regardless of whether it also carried a panel record.
func subagentInternalToolCall(meta map[string]any) bool {
	claudeCode, ok := mapValue(meta["claudeCode"])
	if !ok {
		return false
	}
	return strings.TrimSpace(stringValue(claudeCode["parentToolUseId"])) != ""
}

func providerSubagentsFromMeta(agent string, meta map[string]any, hint providerSubagentHint) []sessionevents.ProviderSubagentEvent {
	if meta == nil {
		return nil
	}
	switch CanonicalAgentName(agent) {
	case AgentCodex:
		provider, ok := mapValue(meta[codexMetaKey])
		if !ok {
			return nil
		}
		raw, ok := provider[providerSubagentsMetaKey]
		if !ok {
			return nil
		}
		return decodeProviderSubagents(raw, agent, hint)
	case AgentClaude:
		provider, ok := mapValue(meta["jaz"])
		if !ok {
			return nil
		}
		raw, ok := provider["providerSubagent"]
		if !ok {
			return nil
		}
		subagent := decodeProviderSubagent(raw, agent, hint)
		if subagent != nil {
			return []sessionevents.ProviderSubagentEvent{*subagent}
		}
	}
	return nil
}

func decodeProviderSubagents(raw any, agent string, hint providerSubagentHint) []sessionevents.ProviderSubagentEvent {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var subagents []sessionevents.ProviderSubagentEvent
	if err := json.Unmarshal(data, &subagents); err != nil {
		return nil
	}
	for i := range subagents {
		if !normalizeProviderSubagent(&subagents[i], agent, hint) {
			return nil
		}
	}
	return subagents
}

func decodeProviderSubagent(raw any, agent string, hint providerSubagentHint) *sessionevents.ProviderSubagentEvent {
	data, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var subagent sessionevents.ProviderSubagentEvent
	if err := json.Unmarshal(data, &subagent); err != nil {
		return nil
	}
	if !normalizeProviderSubagent(&subagent, agent, hint) {
		return nil
	}
	return &subagent
}

func normalizeProviderSubagent(subagent *sessionevents.ProviderSubagentEvent, agent string, hint providerSubagentHint) bool {
	subagent.ID = strings.TrimSpace(subagent.ID)
	subagent.ThreadID = strings.TrimSpace(subagent.ThreadID)
	if subagent.ID == "" {
		subagent.ID = subagent.ThreadID
	}
	if subagent.ID == "" {
		return false
	}
	if subagent.Provider == "" {
		subagent.Provider = CanonicalAgentName(agent)
	}
	if subagent.Status == "" {
		subagent.Status = hint.status
	}
	if subagent.Summary == "" {
		subagent.Summary = hint.summary
	}
	return true
}

func mapValue(value any) (map[string]any, bool) {
	out, ok := value.(map[string]any)
	return out, ok
}
