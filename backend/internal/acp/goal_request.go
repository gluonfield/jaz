package acp

import (
	"strings"
	"time"
)

const jazGoalPrompt = `<jaz_goal_mode>
Goal mode is active for this turn.
Before doing substantive work, call get_goal. If no active goal exists, call create_goal with the concise objective you will pursue. Do not copy the raw user message unless it is already the exact objective.
Set token_budget only when the user explicitly provided a token budget for this goal. Never estimate or invent one. The budget limits automatic continuation after a turn; it does not interrupt a turn already in progress. Jaz tracks tokens_used as uncached input plus output from usage events after create_goal.
Use get_goal when you need current goal usage. When the objective is achieved, call update_goal with status "complete"; if progress is impossible without user input or an external change, call update_goal with status "blocked".
</jaz_goal_mode>`

// Sent with GoalRequested, so goalPromptMessage prepends the jaz_goal_mode block
// documenting the goal tools; this text only steers the agent to keep going.
const jazGoalContinuationMessage = `Continue working toward the active goal. Call get_goal to check the objective and usage. This goal persists across turns: keep the full objective intact and make concrete progress toward the real requested end state rather than redefining success as a smaller task. When the objective is fully achieved, call update_goal with status "complete"; if you cannot progress without user input or an external change, call update_goal with status "blocked"; otherwise keep going now.`

func goalPromptMessage(message string, requested bool) string {
	if !requested {
		return message
	}
	if strings.TrimSpace(message) == "" {
		return jazGoalPrompt
	}
	return jazGoalPrompt + "\n\n" + message
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

func activeGoalTurnStartedAt(job *jobState) (time.Time, bool) {
	job.mu.RLock()
	defer job.mu.RUnlock()
	if job.turn == nil || !job.turn.goalRequested {
		return time.Time{}, false
	}
	return job.turn.startedAt, true
}

func (j *jobState) setTurnGoalRequested() {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.turn != nil {
		j.turn.goalRequested = true
	}
}
