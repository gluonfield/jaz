package acp

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	acpMethodGoalUpdate  = "_jaz/session_goal_update"
	acpSessionUpdateGoal = "_jaz_goal_update"
)

type goalUpdateEnvelope struct {
	SessionUpdate string            `json:"sessionUpdate"`
	Goal          goalUpdatePayload `json:"goal"`
}

type goalNotificationEnvelope struct {
	SessionID string            `json:"sessionId"`
	Goal      goalUpdatePayload `json:"goal"`
}

type goalUpdatePayload struct {
	ThreadID        string `json:"threadId"`
	Objective       string `json:"objective"`
	Status          string `json:"status"`
	TokenBudget     *int64 `json:"tokenBudget"`
	TokensUsed      int64  `json:"tokensUsed"`
	TimeUsedSeconds int64  `json:"timeUsedSeconds"`
	CreatedAt       int64  `json:"createdAt"`
	UpdatedAt       int64  `json:"updatedAt"`
}

func decodeGoalUpdate(raw json.RawMessage) (sessionevents.GoalEvent, bool) {
	var env goalUpdateEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || env.SessionUpdate != acpSessionUpdateGoal {
		return sessionevents.GoalEvent{}, false
	}
	return goalEventFromPayload(env.Goal)
}

func decodeGoalNotification(raw json.RawMessage) (string, sessionevents.GoalEvent, bool) {
	var env goalNotificationEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || env.SessionID == "" {
		return "", sessionevents.GoalEvent{}, false
	}
	goal, ok := goalEventFromPayload(env.Goal)
	return env.SessionID, goal, ok
}

func goalEventFromPayload(payload goalUpdatePayload) (sessionevents.GoalEvent, bool) {
	status := normalizeGoalStatus(payload.Status)
	if status == "" {
		return sessionevents.GoalEvent{}, false
	}
	if payload.TokensUsed < 0 || payload.TimeUsedSeconds < 0 || (payload.TokenBudget != nil && *payload.TokenBudget < 0) {
		return sessionevents.GoalEvent{}, false
	}
	var remainingTokens *int64
	if payload.TokenBudget != nil {
		remaining := *payload.TokenBudget - payload.TokensUsed
		if remaining < 0 {
			remaining = 0
		}
		remainingTokens = &remaining
	}
	return sessionevents.GoalEvent{
		ThreadID:        payload.ThreadID,
		Objective:       strings.TrimSpace(payload.Objective),
		Status:          status,
		TokenBudget:     payload.TokenBudget,
		TokensUsed:      payload.TokensUsed,
		RemainingTokens: remainingTokens,
		TimeUsedSeconds: payload.TimeUsedSeconds,
		CreatedAt:       unixGoalSeconds(payload.CreatedAt),
		UpdatedAt:       unixGoalSeconds(payload.UpdatedAt),
	}, true
}

func (m *Manager) publishGoalUpdate(job *jobState, goal sessionevents.GoalEvent) {
	now := time.Now().UTC()
	if goal.CreatedAt.IsZero() {
		goal.CreatedAt = now
	}
	if goal.UpdatedAt.IsZero() {
		goal.UpdatedAt = now
	}
	job.mu.Lock()
	job.UpdatedAt = now
	job.LastEventAt = now
	job.mu.Unlock()
	snapshot := job.Snapshot()
	events := make([]sessionevents.Event, 0, len(surfaceSessionIDs(&snapshot)))
	for _, sessionID := range surfaceSessionIDs(&snapshot) {
		goalCopy := goal
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      sessionevents.TypeGoalUpdate,
			Goal:      &goalCopy,
			At:        now,
		})
	}
	m.publishOrderedACPEvents(snapshot, events...)
}

func normalizeGoalStatus(status string) string {
	status = strings.TrimSpace(status)
	switch status {
	case sessionevents.GoalStatusActive,
		sessionevents.GoalStatusPaused,
		sessionevents.GoalStatusBlocked,
		sessionevents.GoalStatusUsageLimited,
		sessionevents.GoalStatusBudgetLimited,
		sessionevents.GoalStatusComplete:
		return status
	default:
		return ""
	}
}

func unixGoalSeconds(n int64) time.Time {
	if n <= 0 {
		return time.Time{}
	}
	return time.Unix(n, 0).UTC()
}
