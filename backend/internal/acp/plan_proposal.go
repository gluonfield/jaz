package acp

import (
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

// publishPlanTurnResult closes out a deferred plan turn (Codex/native): publish
// the proposed plan if the agent produced one, otherwise treat the buffered
// assistant message as the plan text awaiting approval.
func (m *Manager) publishPlanTurnResult(job Job) {
	if m.publishProposedPlan(job) {
		return
	}
	if explanation := strings.TrimSpace(job.Assistant); explanation != "" {
		m.publishProposedPlanText(job, explanation)
	}
}

// publishProposedPlan emits the plan the agent built during a plan turn. Codex
// relays it as `plan` session updates accumulated into job.Plan; returns false
// when the turn produced no plan so the caller falls back to the message.
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

func (m *Manager) publishProposedPlanText(job Job, explanation string) {
	m.publishPlanEvent(job, sessionevents.PlanEvent{
		Explanation:      explanation,
		AwaitingApproval: true,
	})
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
