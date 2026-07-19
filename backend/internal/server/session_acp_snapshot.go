package server

import (
	"sort"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type acpMetaEntry struct {
	Title           string `json:"title,omitempty"`
	Slug            string `json:"slug,omitempty"`
	ModelProvider   string `json:"model_provider,omitempty"`
	Model           string `json:"model,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
}

// Stored events carry only the acp session id and slug; session-constant
// labels live here once per response instead of per row.
func (s *Server) acpMeta(events []sessionevents.Event, session storage.Session, children []storage.ACPState) map[string]acpMetaEntry {
	ids := map[string]bool{}
	for _, event := range events {
		if event.ACP != nil && event.ACP.ID != "" {
			ids[event.ACP.ID] = true
		}
	}
	childByID := make(map[string]storage.ACPState, len(children))
	for _, child := range children {
		childByID[child.ID] = child
	}
	meta := make(map[string]acpMetaEntry, len(ids))
	for id := range ids {
		if id == session.ID {
			meta[id] = acpMetaFromSession(session)
			continue
		}
		if child, ok := childByID[id]; ok {
			meta[id] = acpMetaFromState(child)
			continue
		}
		if ref, err := s.Store.LoadSession(id); err == nil {
			meta[id] = acpMetaFromSession(ref)
		}
	}
	return meta
}

func acpMetaFromSession(session storage.Session) acpMetaEntry {
	return acpMetaEntry{
		Title:           session.Title,
		Slug:            session.Slug,
		ModelProvider:   session.ModelProvider,
		Model:           session.Model,
		ReasoningEffort: session.ReasoningEffort,
	}
}

func acpMetaFromState(state storage.ACPState) acpMetaEntry {
	return acpMetaEntry{
		Title:           state.Title,
		Slug:            state.Slug,
		ModelProvider:   state.ModelProvider,
		Model:           state.Model,
		ReasoningEffort: state.ReasoningEffort,
	}
}

func (s *Server) acpSnapshot(session storage.Session) (storage.ACPState, bool) {
	if session.Status == storage.StatusError {
		return canonicalACPStateResponse(s.failedACPSnapshot(session)), true
	}
	if s.ACP != nil {
		if job, err := s.ACP.Status(session.ID); err == nil && job.State != acp.StateNotRunning {
			return canonicalACPStateResponse(acpStateWithSessionLabels(session, acpJobState(job))), true
		}
	}
	if state, err := s.Store.LoadACPState(session.ID); err == nil {
		return canonicalACPStateResponse(inactiveACPStateResponse(acpStateWithSessionFallbackRuntime(session, state))), true
	}
	if s.ACP != nil {
		if job, err := s.ACP.Status(session.ID); err == nil {
			return canonicalACPStateResponse(inactiveACPStateResponse(acpStateWithSessionFallbackRuntime(session, acpJobState(job)))), true
		}
	}
	if session.Runtime == storage.RuntimeACP {
		return canonicalACPStateResponse(inactiveACPStateResponse(acpStateFromSession(session))), true
	}
	return storage.ACPState{}, false
}

func acpStateWithSessionLabels(session storage.Session, state storage.ACPState) storage.ACPState {
	state.ID = firstNonEmpty(state.ID, session.ID)
	state.Slug = firstNonEmpty(session.Slug, state.Slug)
	state.Title = firstNonEmpty(session.Title, state.Title)
	state.ParentID = firstNonEmpty(session.ParentID, state.ParentID)
	state.ModelProvider = firstNonEmpty(session.ModelProvider, state.ModelProvider)
	state.Model = firstNonEmpty(session.Model, state.Model)
	state.ReasoningEffort = firstNonEmpty(session.ReasoningEffort, state.ReasoningEffort)
	if state.CreatedAt.IsZero() {
		state.CreatedAt = session.CreatedAt
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = session.UpdatedAt
	}
	return state
}

func acpStateWithSessionFallbackRuntime(session storage.Session, state storage.ACPState) storage.ACPState {
	state = acpStateWithSessionLabels(session, state)
	if session.RuntimeRef != nil {
		state.ACPAgent = firstNonEmpty(state.ACPAgent, session.RuntimeRef.Agent)
		state.ACPSession = firstNonEmpty(state.ACPSession, session.RuntimeRef.SessionID)
		state.Cwd = firstNonEmpty(state.Cwd, session.RuntimeRef.Cwd)
	}
	return state
}

func (s *Server) failedACPSnapshot(session storage.Session) storage.ACPState {
	state, err := s.Store.LoadACPState(session.ID)
	if err != nil {
		state = acpStateFromSession(session)
	} else {
		state = inactiveACPStateResponse(acpStateWithSessionFallbackRuntime(session, state))
		state.CreatedAt = session.CreatedAt
		state.UpdatedAt = session.UpdatedAt
	}
	state.State = acp.StateFailed
	state.Error = session.Error
	state.Permissions = nil
	return state
}

type parentChildACPView struct {
	state       storage.ACPState
	permissions []sessionevents.ACPPermission
}

func (s *Server) acpChildSnapshots(parentID string) ([]storage.ACPState, []sessionevents.ACPPermission) {
	byID := map[string]parentChildACPView{}
	errorChild := map[string]bool{}
	children, err := s.Store.ListSessions(storage.SessionFilter{
		ParentID:   parentID,
		ParentOnly: true,
		Runtime:    storage.RuntimeACP,
	})
	if err == nil {
		for _, child := range children {
			errorChild[child.ID] = child.Status == storage.StatusError
			if state, ok := s.acpSnapshot(child); ok {
				byID[state.ID] = parentChildACPView{
					state:       parentChildSnapshot(state),
					permissions: activeChildPermissions(state),
				}
			}
		}
	}
	if s.ACP != nil {
		for _, job := range s.ACP.List() {
			if errorChild[job.ID] {
				continue
			}
			if job.ParentID == parentID {
				state := canonicalACPStateResponse(acpJobState(job))
				byID[job.ID] = parentChildACPView{
					state:       parentChildSnapshot(state),
					permissions: activeChildPermissions(state),
				}
			}
		}
	}
	out := make([]storage.ACPState, 0, len(byID))
	var permissions []sessionevents.ACPPermission
	for _, view := range byID {
		out = append(out, view.state)
		permissions = append(permissions, view.permissions...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	sort.Slice(permissions, func(i, j int) bool {
		return permissions[i].ID < permissions[j].ID
	})
	return out, permissions
}

func parentChildSnapshot(state storage.ACPState) storage.ACPState {
	state.Assistant = ""
	state.Thought = ""
	state.Plan = nil
	state.ToolCalls = nil
	state.Permissions = nil
	return state
}

func activeChildPermissions(state storage.ACPState) []sessionevents.ACPPermission {
	if len(state.Permissions) == 0 {
		return nil
	}
	out := make([]sessionevents.ACPPermission, 0, len(state.Permissions))
	for _, permission := range state.Permissions {
		if permission.ID != "" && permission.Status != "selected" && permission.Status != "cancelled" && permission.Status != "canceled" {
			out = append(out, permission)
		}
	}
	return out
}

func applyACPStateResponse(resp map[string]any, state storage.ACPState) {
	state = canonicalACPStateResponse(state)
	resp["acp_state"] = state.State
	resp["acp_assistant"] = state.Assistant
	resp["acp_thought"] = state.Thought
	resp["acp_modes"] = state.Modes
	resp["acp_plan"] = state.Plan
	resp["acp_tool_calls"] = state.ToolCalls
	resp["acp_permissions"] = state.Permissions
	resp["acp_error"] = state.Error
	resp["acp_goal_requested"] = state.GoalRequested
	resp["acp_active_operation"] = state.ActiveOperation
	resp["acp_last_event_at"] = state.LastEventAt
	resp["acp_last_tool_at"] = state.LastToolAt
}

func canonicalACPStateResponse(state storage.ACPState) storage.ACPState {
	if canonical := acp.CanonicalAgentName(state.ACPAgent); canonical != "" {
		state.ACPAgent = canonical
	}
	return state
}

func inactiveACPStateResponse(state storage.ACPState) storage.ACPState {
	if state.State == acp.StateStarting || state.State == acp.StateRunning || state.State == acp.StateNotRunning {
		state.State = acp.StateIdle
	}
	state.GoalRequested = false
	state.ActiveOperation = ""
	return state.WithoutTranscript()
}

func acpJobState(job acp.Job) storage.ACPState {
	plan := make([]sessionevents.ACPPlanEntry, 0, len(job.Plan))
	for _, entry := range job.Plan {
		plan = append(plan, sessionevents.ACPPlanEntry{
			Content:  entry.Content,
			Status:   entry.Status,
			Priority: entry.Priority,
		})
	}
	return storage.ACPState{
		ID:              job.ID,
		Slug:            job.Slug,
		Title:           job.Title,
		ParentID:        job.ParentID,
		ACPAgent:        acp.CanonicalAgentName(job.ACPAgent),
		ACPSession:      job.ACPSession,
		Cwd:             job.Cwd,
		ModelProvider:   job.ModelProvider,
		Model:           job.Model,
		ReasoningEffort: job.ReasoningEffort,
		State:           job.State,
		StopReason:      job.StopReason,
		Assistant:       job.Assistant,
		Thought:         job.Thought,
		Plan:            plan,
		ToolCalls:       acp.CloneToolCalls(job.ToolCalls),
		Permissions:     job.Permissions,
		Modes: sessionevents.ACPModeState{
			CurrentModeID:  job.Modes.CurrentModeID,
			PlanModeID:     job.Modes.PlanModeID,
			AvailableModes: acpModes(job.Modes.AvailableModes),
		},
		Error:           job.Error,
		GoalRequested:   job.GoalRequested,
		ActiveOperation: job.ActiveOperation,
		ParentVisible:   job.ParentVisible,
		CreatedAt:       job.CreatedAt,
		UpdatedAt:       job.UpdatedAt,
		LastEventAt:     job.LastEventAt,
		LastToolAt:      job.LastToolAt,
	}
}

func acpModes(in []acp.ModeSnapshot) []sessionevents.ACPMode {
	out := make([]sessionevents.ACPMode, 0, len(in))
	for _, mode := range in {
		out = append(out, sessionevents.ACPMode{
			ID:          mode.ID,
			Name:        mode.Name,
			Description: mode.Description,
		})
	}
	return out
}

func acpStateFromSession(session storage.Session) storage.ACPState {
	session = canonicalSession(session)
	state := storage.ACPState{
		ID:              session.ID,
		Slug:            session.Slug,
		Title:           session.Title,
		ParentID:        session.ParentID,
		ModelProvider:   session.ModelProvider,
		Model:           session.Model,
		ReasoningEffort: session.ReasoningEffort,
		State:           acp.StateNotRunning,
		CreatedAt:       session.CreatedAt,
		UpdatedAt:       session.UpdatedAt,
	}
	if session.RuntimeRef != nil {
		state.ACPAgent = session.RuntimeRef.Agent
		state.ACPSession = session.RuntimeRef.SessionID
		state.Cwd = session.RuntimeRef.Cwd
	}
	return state
}
