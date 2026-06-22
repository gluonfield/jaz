package acp

import (
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func jobFromInactiveState(session storage.Session, state storage.ACPState) Job {
	agent := state.ACPAgent
	acpSession := state.ACPSession
	if session.RuntimeRef != nil {
		agent = firstNonEmpty(agent, session.RuntimeRef.Agent)
		acpSession = firstNonEmpty(acpSession, session.RuntimeRef.SessionID)
	}
	return Job{
		ID:              firstNonEmpty(state.ID, session.ID),
		Slug:            firstNonEmpty(state.Slug, session.Slug),
		Title:           firstNonEmpty(state.Title, session.Title),
		ParentID:        firstNonEmpty(state.ParentID, session.ParentID),
		ACPAgent:        CanonicalAgentName(agent),
		ACPSession:      acpSession,
		Cwd:             state.Cwd,
		ModelProvider:   firstNonEmpty(session.ModelProvider, state.ModelProvider),
		Model:           firstNonEmpty(session.Model, state.Model),
		ReasoningEffort: firstNonEmpty(session.ReasoningEffort, state.ReasoningEffort),
		State:           StateNotRunning,
		StopReason:      state.StopReason,
		Assistant:       state.Assistant,
		Thought:         state.Thought,
		Plan:            clonePlanEntries(state.Plan),
		ToolCalls:       CloneToolCalls(state.ToolCalls),
		Modes:           modeStateFromEvent(state.Modes),
		Error:           state.Error,
		CreatedAt:       firstNonZeroTime(state.CreatedAt, session.CreatedAt),
		UpdatedAt:       firstNonZeroTime(state.UpdatedAt, session.UpdatedAt),
		LastEventAt:     firstNonZeroTime(state.LastEventAt, state.UpdatedAt),
		LastToolAt:      state.LastToolAt,
	}
}

func modeStateFromEvent(modes sessionevents.ACPModeState) ModeState {
	out := ModeState{
		CurrentModeID:  modes.CurrentModeID,
		PlanModeID:     modes.PlanModeID,
		AvailableModes: make([]ModeSnapshot, 0, len(modes.AvailableModes)),
	}
	for _, mode := range modes.AvailableModes {
		out.AvailableModes = append(out.AvailableModes, ModeSnapshot{
			ID:          mode.ID,
			Name:        mode.Name,
			Description: mode.Description,
		})
	}
	return out
}

func firstNonZeroTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}
