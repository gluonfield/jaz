package acp

import (
	"errors"
	"strings"

	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
)

var ErrNativeGoalUnsupported = errors.New("acp native goal unsupported")

func goalPromptMeta(requested bool) map[string]any {
	if !requested {
		return nil
	}
	return map[string]any{
		jazMetaKey: map[string]any{"goalRequested": true},
	}
}

func (m *Manager) recordGoalRequest(job *jobState, requested bool, objective string) {
	if !requested {
		return
	}
	job.setTurnGoalRequested()
	m.publishGoalUpdate(job, sessionevents.GoalEvent{
		Identity: goal.Identity{
			Objective: strings.TrimSpace(objective),
			Status:    sessionevents.GoalStatusRequested,
		},
	})
}

func currentTurnGoalRequested(job *jobState, done chan struct{}) bool {
	job.mu.RLock()
	defer job.mu.RUnlock()
	return job.turn != nil && job.turn.done == done && job.turn.goalRequested
}

func (j *jobState) setTurnGoalRequested() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.turn != nil {
		j.turn.goalRequested = true
	}
}

func (j *jobState) supportsNativeGoal() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.nativeGoal
}
