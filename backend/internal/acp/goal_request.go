package acp

import (
	"errors"
)

var ErrNativeGoalUnsupported = errors.New("acp native goal unsupported")

func goalPromptMeta(requested bool) map[string]any {
	if !requested {
		return nil
	}
	goal := map[string]any{"requested": true}
	return map[string]any{
		codexMetaKey: map[string]any{"goal": goal},
	}
}

func markGoalRequested(job *jobState, requested bool) {
	if !requested {
		return
	}
	job.setTurnGoalRequested()
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
