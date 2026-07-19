package acp

import (
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

// publishPlanTurnResult closes out a deferred plan turn: publish the proposed
// plan through the shared PlanEvent shape.
func (m *Manager) publishPlanTurnResult(job Job, proposal *sessionevents.PlanEvent) {
	if proposal != nil {
		m.publishPlanEvent(job, *proposal)
	}
}

func (m *Manager) publishPlanEvent(job Job, plan sessionevents.PlanEvent) {
	acp := EventFromJob(job)
	acp.Plan = nil

	events := make([]sessionevents.Event, 0, len(surfaceSessionIDs(&job)))
	for _, sessionID := range surfaceSessionIDs(&job) {
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      "proposed_plan",
			ACP:       acp,
			Plan:      &plan,
			At:        time.Now().UTC(),
		})
	}
	m.publishOrderedACPEvents(job, events...)
}

func clonePlanEvent(in *sessionevents.PlanEvent) *sessionevents.PlanEvent {
	if in == nil {
		return nil
	}
	out := *in
	out.Plan = clonePlanEntries(in.Plan)
	return &out
}
