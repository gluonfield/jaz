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

// providerSubagentFromUpdate publishes subagent panel records and decides which
// updates to keep out of the main transcript. Both providers report subagents
// the same way — a _meta.jaz.providerSubagent record built at the adapter edge
// (codex-acp's collab_subagents, claude-agent-acp's subagents) — so publishing
// is provider-agnostic. Consuming is Jaz display policy: codex subagent records
// ride dedicated session_info updates, while Claude streams a subagent's own
// tool calls inline tagged with claudeCode.parentToolUseId; those belong to the
// subagent's panel, never the parent turn.
func providerSubagentFromUpdate(agent string, update acpschema.DecodedSessionUpdate) providerSubagentUpdate {
	switch event := update.(type) {
	case acpschema.SessionInfoSessionUpdate:
		subagent := providerSubagentFromJazMeta(agent, event.Meta, providerSubagentHint{})
		return providerSubagentUpdate{subagent: subagent, consume: subagent != nil}
	case acpschema.ToolCallSessionUpdate:
		return toolCallSubagent(agent, event.Meta)
	case acpschema.ToolCallUpdateSessionUpdate:
		return toolCallSubagent(agent, event.Meta)
	case acpschema.AgentMessageChunkUpdate:
		return providerSubagentUpdate{subagent: providerSubagentFromJazMeta(agent, event.Meta, providerSubagentHint{summary: "Subagent message", status: "running"})}
	case acpschema.AgentThoughtChunkUpdate:
		return providerSubagentUpdate{subagent: providerSubagentFromJazMeta(agent, event.Meta, providerSubagentHint{summary: "Subagent thinking", status: "running"})}
	default:
		return providerSubagentUpdate{}
	}
}

func toolCallSubagent(agent string, meta map[string]any) providerSubagentUpdate {
	return providerSubagentUpdate{
		subagent: providerSubagentFromJazMeta(agent, meta, providerSubagentHint{status: "running"}),
		consume:  subagentInternalToolCall(meta),
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
