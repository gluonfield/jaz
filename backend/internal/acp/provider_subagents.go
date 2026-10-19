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
	status  sessionevents.ProviderSubagentStatus
}

type providerSubagentUpdate struct {
	subagents []sessionevents.ProviderSubagentEvent
	consume   bool
}

// providerSubagentFromUpdate publishes subagent panel records and decides which
// updates to keep out of the main transcript.
func providerSubagentFromUpdate(job *jobState, update acpschema.DecodedSessionUpdate) providerSubagentUpdate {
	agent := job.ACPAgent
	switch event := update.(type) {
	case acpschema.SessionInfoSessionUpdate:
		subagents := providerSubagentsFromMeta(agent, event.Meta, providerSubagentHint{})
		return providerSubagentUpdate{subagents: subagents, consume: len(subagents) > 0}
	case acpschema.ToolCallSessionUpdate:
		return job.toolCallSubagent(event.ToolCallID, event.Meta)
	case acpschema.ToolCallUpdateSessionUpdate:
		return job.toolCallSubagent(event.ToolCallID, event.Meta)
	case acpschema.AgentMessageChunkUpdate:
		return providerSubagentUpdate{subagents: providerSubagentsFromMeta(agent, event.Meta, providerSubagentHint{summary: "Subagent message", status: sessionevents.SubagentRunning})}
	case acpschema.AgentThoughtChunkUpdate:
		return providerSubagentUpdate{subagents: providerSubagentsFromMeta(agent, event.Meta, providerSubagentHint{summary: "Subagent thinking", status: sessionevents.SubagentRunning})}
	default:
		return providerSubagentUpdate{}
	}
}

// toolCallSubagent publishes a tool call's subagent record and keeps a Claude
// subagent's own tool calls out of the main transcript. Claude marks only a
// call's first report with claudeCode.parentToolUseId, so the job remembers the id.
func (j *jobState) toolCallSubagent(id acpschema.ToolCallID, meta map[string]any) providerSubagentUpdate {
	j.mu.Lock()
	defer j.mu.Unlock()
	claudeCode, _ := mapValue(meta["claudeCode"])
	if strings.TrimSpace(stringValue(claudeCode["parentToolUseId"])) != "" {
		if j.subagentToolIDs == nil {
			j.subagentToolIDs = make(map[string]struct{})
		}
		j.subagentToolIDs[string(id)] = struct{}{}
	}
	_, nested := j.subagentToolIDs[string(id)]
	return providerSubagentUpdate{
		subagents: providerSubagentsFromMeta(j.ACPAgent, meta, providerSubagentHint{status: sessionevents.SubagentRunning}),
		consume:   nested,
	}
}

// interruptProviderSubagents closes the subagents the session's agent process
// left open, since no process remains to report their end.
func (m *Manager) interruptProviderSubagents(job *jobState) {
	open, err := m.openProviderSubagents(job.ID)
	if err != nil {
		m.log.Error("load provider subagents", "session", job.ID, "error", err)
		return
	}
	interrupted := make([]sessionevents.ProviderSubagentEvent, 0, len(open))
	for _, subagent := range open {
		interrupted = append(interrupted, sessionevents.ProviderSubagentEvent{Provider: subagent.Provider, ID: subagent.ID, Status: sessionevents.SubagentCancelled})
	}
	m.publishProviderSubagents(job.eventView(), interrupted)
}

// openProviderSubagents lists the session's own subagents that have not
// reported an end.
func (m *Manager) openProviderSubagents(id string) ([]sessionevents.ProviderSubagentEvent, error) {
	events, err := m.store.LoadSessionOverviewEvents(id)
	if err != nil {
		return nil, err
	}
	var open []sessionevents.ProviderSubagentEvent
	for _, event := range events {
		subagent := event.ProviderSubagent
		if subagent != nil && event.ProjectionKey == sessionevents.ProviderSubagentProjectionKey(id, *subagent) && !subagent.Status.Ended() {
			open = append(open, *subagent)
		}
	}
	return open, nil
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
