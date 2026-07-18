package acp

import (
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

func (m *Manager) publishPlanEvent(job Job, plan sessionevents.PlanEvent) {
	acp := EventFromJob(job)
	acp.Assistant = ""
	acp.Thought = ""
	acp.Plan = nil
	acp.ToolCalls = nil
	acp.Permissions = nil

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
