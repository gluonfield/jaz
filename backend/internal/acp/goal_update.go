package acp

import (
	"encoding/json"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

const (
	acpMethodGoalUpdate       = "_jaz/session_goal_update"
	acpMethodGoalClear        = "_jaz/session_goal_clear"
	acpMethodCodexGoalUpdated = "thread/goal/updated"
	acpMethodCodexGoalCleared = "thread/goal/cleared"
	acpSessionUpdateGoal      = "_jaz_goal_update"
	acpSessionUpdateGoalClear = "_jaz_goal_clear"
)

type goalUpdateEnvelope struct {
	SessionUpdate string `json:"sessionUpdate"`
}

type goalClearUpdateEnvelope struct {
	SessionUpdate string `json:"sessionUpdate"`
}

func isGoalUpdate(raw json.RawMessage) bool {
	var env goalUpdateEnvelope
	if err := json.Unmarshal(raw, &env); err != nil || env.SessionUpdate != acpSessionUpdateGoal {
		return false
	}
	return true
}

func isGoalClearUpdate(raw json.RawMessage) bool {
	var env goalClearUpdateEnvelope
	return json.Unmarshal(raw, &env) == nil && env.SessionUpdate == acpSessionUpdateGoalClear
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
