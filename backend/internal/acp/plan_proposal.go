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
		return
	}
	if m.publishProposedPlan(job) {
		return
	}
}

// publishProposedPlan emits the plan the agent built during a plan turn. Codex
// can relay it as structured `plan` session updates accumulated into job.Plan.
func (m *Manager) publishProposedPlan(job Job) bool {
	plan := clonePlanEntries(job.Plan)
	if len(plan) == 0 {
		return false
	}
	m.publishPlanEvent(job, sessionevents.PlanEvent{
		Plan:             plan,
		AwaitingApproval: true,
	})
	return true
}

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

func clonePlanEvent(in *sessionevents.PlanEvent) *sessionevents.PlanEvent {
	if in == nil {
		return nil
	}
	out := *in
	out.Plan = clonePlanEntries(in.Plan)
	return &out
}
