package acp

import (
	"sync"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

type eventProjection struct {
	mu        sync.Mutex
	annotator sessionevents.Annotator
}

func (m *Manager) touchJobAttention(job *jobState) {
	snapshot := job.eventView()
	m.touchAttention(surfaceSessionIDs(snapshot)...)
}

func (m *Manager) touchAttention(sessionIDs ...string) {
	seen := map[string]bool{}
	for _, sessionID := range sessionIDs {
		if sessionID == "" || seen[sessionID] {
			continue
		}
		seen[sessionID] = true
		_ = m.store.TouchSessionAttention(sessionID)
	}
}

func (m *Manager) publishSessionChanged(sessionID string) {
	if m.Events == nil || sessionID == "" {
		return
	}
	m.Events.Publish(sessionevents.Event{SessionID: sessionID, Type: sessionevents.TypeSession})
}

func (m *Manager) publishACP(job eventView) {
	acp := eventFromView(job)
	events := make([]sessionevents.Event, 0, 2)
	for _, sessionID := range childSessionIDs(job) {
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      "acp",
			ACP:       acp,
			At:        time.Now().UTC(),
		})
	}
	for _, sessionID := range parentSessionIDs(job) {
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      "acp",
			ACP:       acp,
			At:        time.Now().UTC(),
		})
	}
	m.publishOrderedACPEvents(job, events...)
}

func (m *Manager) publishACPStatus(job eventView) {
	acp := eventFromView(job)
	acp.Plan = nil
	events := make([]sessionevents.Event, 0, len(surfaceSessionIDs(job)))
	for _, sessionID := range surfaceSessionIDs(job) {
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      "acp",
			ACP:       acp,
			At:        time.Now().UTC(),
		})
	}
	m.publishOrderedACPEvents(job, events...)
}

func (m *Manager) publishACPTool(job eventView, call sessionevents.ACPToolCall) {
	acp := acpEventEnvelope(job)
	acp.ToolCalls = CloneToolCalls([]sessionevents.ACPToolCall{call})
	events := make([]sessionevents.Event, 0, len(childSessionIDs(job)))
	for _, sessionID := range childSessionIDs(job) {
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      "acp_tool",
			ACP:       acp,
			At:        time.Now().UTC(),
		})
	}
	m.publishOrderedACPEvents(job, events...)
}

func (m *Manager) publishProviderSubagent(job eventView, subagent sessionevents.ProviderSubagentEvent) {
	if subagent.ID == "" {
		return
	}
	if subagent.Provider == "" {
		subagent.Provider = CanonicalAgentName(job.ACPAgent)
	}
	events := make([]sessionevents.Event, 0, len(surfaceSessionIDs(job)))
	for _, sessionID := range surfaceSessionIDs(job) {
		events = append(events, sessionevents.Event{
			SessionID:        sessionID,
			Type:             sessionevents.TypeProviderSubagent,
			ProviderSubagent: &subagent,
			At:               time.Now().UTC(),
			ProjectionKey:    sessionevents.ProviderSubagentProjectionKey(job.ID, subagent),
		})
	}
	m.publishOrderedACPEvents(job, events...)
}

func (m *Manager) publishOrderedACPEvents(job eventView, events ...sessionevents.Event) {
	m.withACPTranscriptBarrier(job, func() {
		m.recordAndPublishEventListDirect(events)
	})
}

func (m *Manager) recordAndPublishDirect(event sessionevents.Event) {
	m.recordAndPublishEventListDirect([]sessionevents.Event{event})
}

func (m *Manager) recordAndPublishEventListDirect(events []sessionevents.Event) {
	for len(events) > 0 {
		sessionID := events[0].SessionID
		n := 1
		for n < len(events) && events[n].SessionID == sessionID {
			n++
		}
		m.recordAndPublishEventsDirect(sessionID, events[:n])
		events = events[n:]
	}
}

func (m *Manager) recordAndPublishEventsDirect(sessionID string, events []sessionevents.Event) {
	if len(events) == 0 {
		return
	}
	now := time.Now().UTC()
	projection := m.eventProjection(sessionID)
	projection.mu.Lock()
	defer projection.mu.Unlock()
	annotator := projection.annotator
	for _, event := range events {
		if event.Type == sessionevents.TypeProviderSubagent {
			annotator = annotator.Clone()
			break
		}
	}
	storedEvents := make([]sessionevents.Event, len(events))
	for i := range events {
		if events[i].At.IsZero() {
			events[i].At = now
		}
		events[i] = annotator.Annotate(events[i])
		storedEvents[i] = events[i].SlimForStorage()
	}
	if sessionID != "" {
		if err := m.store.AppendSessionEvents(sessionID, storedEvents...); err != nil {
			m.log.Error("persist session events", "session", sessionID, "error", err)
			return
		}
	}
	projection.annotator = annotator
	if m.Events == nil {
		return
	}
	for i := range events {
		events[i].Seq = storedEvents[i].Seq
		if events[i].Type == sessionevents.TypeProviderSubagent {
			events[i].ProviderSubagent = storedEvents[i].ProviderSubagent
			events[i].Content = storedEvents[i].Content
		}
		m.Events.Publish(events[i])
	}
}

func (m *Manager) eventProjection(sessionID string) *eventProjection {
	m.projectionMu.Lock()
	defer m.projectionMu.Unlock()
	if m.projections == nil {
		m.projections = make(map[string]*eventProjection)
	}
	projection := m.projections[sessionID]
	if projection == nil {
		projection = &eventProjection{}
		m.projections[sessionID] = projection
	}
	return projection
}

func EventFromJob(job Job) *sessionevents.ACPEvent {
	return eventFromView(eventViewFromJob(job))
}

func eventFromView(job eventView) *sessionevents.ACPEvent {
	event := acpEventEnvelope(job)
	event.Plan = clonePlanEntries(job.Plan)
	return event
}

func acpEventEnvelope(job eventView) *sessionevents.ACPEvent {
	return &sessionevents.ACPEvent{
		ID:              job.ID,
		Slug:            job.Slug,
		Title:           job.Title,
		ParentID:        job.ParentID,
		Agent:           job.ACPAgent,
		SessionID:       job.ACPSession,
		ModelProvider:   job.ModelProvider,
		Model:           job.Model,
		ReasoningEffort: job.ReasoningEffort,
		State:           job.State,
		StopReason:      job.StopReason,
		Error:           job.Error,
		GoalRequested:   job.GoalRequested,
		Modes:           acpModeEvent(job.Modes),
		LastEventAt:     job.LastEventAt,
		LastToolAt:      job.LastToolAt,
	}
}

func acpModeEvent(modes ModeState) sessionevents.ACPModeState {
	out := sessionevents.ACPModeState{
		CurrentModeID:  modes.CurrentModeID,
		PlanModeID:     modes.PlanModeID,
		AvailableModes: make([]sessionevents.ACPMode, 0, len(modes.AvailableModes)),
	}
	for _, mode := range modes.AvailableModes {
		out.AvailableModes = append(out.AvailableModes, sessionevents.ACPMode{
			ID:          mode.ID,
			Name:        mode.Name,
			Description: mode.Description,
		})
	}
	return out
}

func childSessionIDs(job eventView) []string {
	return []string{job.ID}
}

func parentSessionIDs(job eventView) []string {
	if job.ParentID == "" || job.ParentID == job.ID || !job.ParentVisible {
		return nil
	}
	return []string{job.ParentID}
}

func surfaceSessionIDs(job eventView) []string {
	return append(childSessionIDs(job), parentSessionIDs(job)...)
}
