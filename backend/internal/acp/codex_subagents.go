package acp

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

type codexSubagentMetadata struct {
	Subagent *struct {
		ThreadID string `json:"threadId"`
		Path     string `json:"path"`
		Activity string `json:"activity"`
	} `json:"subagent"`
	Collaboration *struct {
		Tool              string   `json:"tool"`
		SenderThreadID    string   `json:"senderThreadId"`
		ReceiverThreadIDs []string `json:"receiverThreadIds"`
	} `json:"collaboration"`
}

type codexCollaborationInput struct {
	Prompt            string   `json:"prompt"`
	Model             string   `json:"model"`
	ReasoningEffort   string   `json:"reasoningEffort"`
	Status            string   `json:"status"`
	ReceiverThreadIDs []string `json:"receiverThreadIds"`
	AgentsStates      map[string]struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"agentsStates"`
}

func codexSubagentsFromMeta(meta map[string]any, rawInput json.RawMessage) []sessionevents.ProviderSubagentEvent {
	data, err := json.Marshal(meta[codexMetaKey])
	if err != nil {
		return nil
	}
	var metadata codexSubagentMetadata
	if json.Unmarshal(data, &metadata) != nil {
		return nil
	}
	if activity := metadata.Subagent; activity != nil {
		return providerSubagentSlice(codexActivitySubagent(activity.ThreadID, activity.Path, activity.Activity))
	}
	collaboration := metadata.Collaboration
	if collaboration == nil {
		return nil
	}
	var input codexCollaborationInput
	_ = json.Unmarshal(rawInput, &input)
	receiverIDs := input.ReceiverThreadIDs
	if len(receiverIDs) == 0 {
		receiverIDs = collaboration.ReceiverThreadIDs
	}
	if len(receiverIDs) == 0 {
		return nil
	}
	subagents := make([]sessionevents.ProviderSubagentEvent, 0, len(receiverIDs))
	for _, receiverID := range receiverIDs {
		threadID := strings.TrimSpace(receiverID)
		if threadID == "" {
			continue
		}
		status := strings.TrimSpace(input.Status)
		summary := ""
		if state, ok := input.AgentsStates[threadID]; ok {
			status = strings.TrimSpace(state.Status)
			summary = strings.TrimSpace(state.Message)
		}
		status = codexCollaborationStatus(status)
		if summary == "" {
			summary = codexCollaborationSummary(strings.TrimSpace(collaboration.Tool), status)
		}
		subagents = append(subagents, sessionevents.ProviderSubagentEvent{
			Provider:        AgentCodex,
			ID:              threadID,
			ThreadID:        threadID,
			ParentID:        strings.TrimSpace(collaboration.SenderThreadID),
			Status:          status,
			Summary:         summary,
			Prompt:          strings.TrimSpace(input.Prompt),
			Model:           strings.TrimSpace(input.Model),
			ReasoningEffort: strings.TrimSpace(input.ReasoningEffort),
		})
	}
	return subagents
}

func codexActivitySubagent(threadID, agentPath, kind string) *sessionevents.ProviderSubagentEvent {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return nil
	}
	name := path.Base(strings.TrimSpace(agentPath))
	if name == "." || name == "/" {
		name = ""
	}
	status, summary := codexActivityState(strings.TrimSpace(kind))
	return &sessionevents.ProviderSubagentEvent{
		Provider: AgentCodex,
		ID:       threadID,
		ThreadID: threadID,
		Name:     name,
		Task:     name,
		Status:   status,
		Summary:  summary,
	}
}

func codexActivityState(kind string) (string, string) {
	switch kind {
	case "started":
		return "running", "Spawned"
	case "interacted":
		return "running", "Working"
	case "interrupted":
		return "cancelled", "Interrupted"
	default:
		return "", kind
	}
}

func codexCollaborationStatus(status string) string {
	switch status {
	case "pendingInit":
		return "starting"
	case "inProgress":
		return "running"
	case "interrupted":
		return "cancelled"
	case "errored":
		return "failed"
	case "shutdown":
		return "stopped"
	case "notFound":
		return "failed"
	default:
		return status
	}
}

func codexCollaborationSummary(tool, status string) string {
	switch status {
	case "failed":
		return "Failed"
	case "cancelled":
		return "Interrupted"
	}
	inProgress := status == "running" || status == "starting"
	switch tool {
	case "spawnAgent":
		return "Spawned"
	case "sendInput":
		if inProgress {
			return "Working"
		}
		return "Responded"
	case "resumeAgent":
		if inProgress {
			return "Resuming"
		}
		return "Resumed"
	case "wait":
		if inProgress {
			return "Waiting"
		}
		return "Wait finished"
	case "closeAgent":
		if inProgress {
			return "Closing"
		}
		return "Closed"
	default:
		return tool
	}
}
