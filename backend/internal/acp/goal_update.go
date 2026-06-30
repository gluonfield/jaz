package acp

import (
	"encoding/json"
	"time"

	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	acpMethodGoalUpdate       = "_jaz/session_goal_update"
	acpMethodGoalClear        = "_jaz/session_goal_clear"
	acpMethodCodexGoalCleared = "thread/goal/cleared"
	acpSessionUpdateGoal      = "_jaz_goal_update"
	acpSessionUpdateGoalClear = "_jaz_goal_clear"
)

type goalUpdateEnvelope struct {
	SessionUpdate string             `json:"sessionUpdate"`
	Goal          goal.UpdatePayload `json:"goal"`
}

type goalNotificationEnvelope struct {
	SessionID string             `json:"sessionId"`
	Goal      goal.UpdatePayload `json:"goal"`
}

type goalClearUpdateEnvelope struct {
	SessionUpdate string `json:"sessionUpdate"`
}

type goalClearNotificationEnvelope struct {
	SessionID string `json:"sessionId"`
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

func decodeGoalClearUpdate(raw json.RawMessage) bool {
	var env goalClearUpdateEnvelope
	return json.Unmarshal(raw, &env) == nil && env.SessionUpdate == acpSessionUpdateGoalClear
}

func decodeGoalClearNotification(raw json.RawMessage) (string, bool) {
	var env goalClearNotificationEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || env.SessionID == "" {
		return "", false
	}
	return env.SessionID, true
}

func goalEventFromPayload(payload goal.UpdatePayload) (sessionevents.GoalEvent, bool) {
	state := payload.State()
	normalized := goal.NormalizeState(&state)
	if normalized == nil || !goal.CompleteSnapshot(normalized) {
		return sessionevents.GoalEvent{}, false
	}
	return *normalized, true
}

func (m *Manager) publishGoalUpdate(job *jobState, state sessionevents.GoalEvent) {
	now := time.Now().UTC()
	if state.Provider == "" {
		state.Provider = job.ACPAgent
	}
	if state.ActiveOperation == "" {
		job.mu.RLock()
		state.ActiveOperation = job.ActiveOperation
		job.mu.RUnlock()
	}
	if state.CreatedAt.IsZero() {
		state.CreatedAt = now
	}
	if state.UpdatedAt.IsZero() {
		state.UpdatedAt = now
	}
	job.mu.Lock()
	job.UpdatedAt = now
	job.LastEventAt = now
	job.mu.Unlock()
	snapshot := job.Snapshot()
	events := make([]sessionevents.Event, 0, len(surfaceSessionIDs(&snapshot)))
	for _, sessionID := range surfaceSessionIDs(&snapshot) {
		goalCopy := state
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      sessionevents.TypeGoalUpdate,
			Goal:      &goalCopy,
			At:        now,
		})
	}
	m.publishOrderedACPEvents(snapshot, events...)
}

func (m *Manager) publishGoalClear(job *jobState) {
	now := time.Now().UTC()
	job.mu.Lock()
	job.UpdatedAt = now
	job.LastEventAt = now
	job.mu.Unlock()
	snapshot := job.Snapshot()
	events := make([]sessionevents.Event, 0, len(surfaceSessionIDs(&snapshot)))
	for _, sessionID := range surfaceSessionIDs(&snapshot) {
		events = append(events, sessionevents.Event{
			SessionID: sessionID,
			Type:      sessionevents.TypeGoalClear,
			At:        now,
		})
	}
	m.publishOrderedACPEvents(snapshot, events...)
}
