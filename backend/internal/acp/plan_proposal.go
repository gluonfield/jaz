package acp

import (
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	proposedPlanOpenTag  = "<proposed_plan>"
	proposedPlanCloseTag = "</proposed_plan>"
)

func (m *Manager) publishPlanTurnResult(job Job) {
	if m.publishProposedPlan(job) {
		return
	}
	if strings.TrimSpace(job.Assistant) != "" {
		m.publishACPMessage(job, job.Assistant)
	}
}

func (m *Manager) publishProposedPlan(job Job) bool {
	explanation := proposedPlanText(job.Assistant)
	plan := clonePlanEntries(job.Plan)
	if explanation == "" && len(plan) == 0 {
		return false
	}
	acp := EventFromJob(job)
	acp.Assistant = ""
	acp.Thought = ""
	acp.Plan = nil
	acp.ToolCalls = nil
	acp.Permissions = nil

	for _, sessionID := range surfaceSessionIDs(&job) {
		m.recordAndPublish(sessionevents.Event{
			SessionID: sessionID,
			Type:      "proposed_plan",
			ACP:       acp,
			Plan: &sessionevents.PlanEvent{
				Explanation:      explanation,
				Plan:             plan,
				AwaitingApproval: true,
			},
			At: time.Now().UTC(),
		})
	}
	return true
}

func proposedPlanText(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	start := strings.Index(text, proposedPlanOpenTag)
	if start < 0 {
		return ""
	}
	afterOpen := text[start+len(proposedPlanOpenTag):]
	end := strings.Index(afterOpen, proposedPlanCloseTag)
	if end < 0 {
		return ""
	}
	return strings.TrimSpace(afterOpen[:end])
}
