package acp

import (
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
)

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
