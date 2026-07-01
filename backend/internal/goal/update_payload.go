package goal

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type UpdatePayload struct {
	state State
}

func (p *UpdatePayload) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	var state State
	var err error
	if state.ThreadID, err = stringField(fields, "threadId", "thread_id"); err != nil {
		return err
	}
	if state.Provider, err = stringField(fields, "provider"); err != nil {
		return err
	}
	if state.ProviderGoalID, err = stringField(fields, "providerGoalID", "providerGoalId", "provider_goal_id", "goalId", "goal_id"); err != nil {
		return err
	}
	if state.Objective, err = stringField(fields, "objective"); err != nil {
		return err
	}
	status, err := stringField(fields, "status")
	if err != nil {
		return err
	}
	state.Status = NormalizeStatus(status)
	if state.TokenBudget, err = intPtrField(fields, "tokenBudget", "token_budget"); err != nil {
		return err
	}
	if state.TokensUsed, err = intField(fields, "tokensUsed", "tokens_used"); err != nil {
		return err
	}
	if state.RemainingTokens, err = intPtrField(fields, "remainingTokens", "remaining_tokens"); err != nil {
		return err
	}
	if state.TimeUsedSeconds, err = intField(fields, "timeUsedSeconds", "time_used_seconds"); err != nil {
		return err
	}
	if state.TurnCount, err = intField(fields, "turnCount", "turn_count"); err != nil {
		return err
	}
	if state.EvaluatedTurns, err = intField(fields, "evaluatedTurns", "evaluated_turns"); err != nil {
		return err
	}
	if state.AttemptCount, err = intField(fields, "attemptCount", "attempt_count"); err != nil {
		return err
	}
	if state.ProgressMessage, err = stringField(fields, "progressMessage", "progress_message"); err != nil {
		return err
	}
	if state.BlockedReason, err = stringField(fields, "blockedReason", "blocked_reason"); err != nil {
		return err
	}
	if state.EvaluatorReason, err = stringField(fields, "evaluatorReason", "evaluator_reason"); err != nil {
		return err
	}
	if state.CompletionReview, err = stringField(fields, "completionReview", "completion_review"); err != nil {
		return err
	}
	if state.ActiveSubagentID, err = stringField(fields, "activeSubagentID", "activeSubagentId", "active_subagent_id"); err != nil {
		return err
	}
	if state.ActiveOperation, err = stringField(fields, "activeOperation", "active_operation"); err != nil {
		return err
	}
	if state.CreatedAt, err = unixSecondsField(fields, "createdAt", "created_at"); err != nil {
		return err
	}
	if state.UpdatedAt, err = unixSecondsField(fields, "updatedAt", "updated_at"); err != nil {
		return err
	}
	if state.CompletedAt, err = unixSecondsField(fields, "completedAt", "completed_at"); err != nil {
		return err
	}
	p.state = state
	return nil
}

func (p UpdatePayload) State() State {
	return p.state
}

func rawField(fields map[string]json.RawMessage, aliases ...string) (json.RawMessage, string, bool) {
	for _, alias := range aliases {
		if raw, ok := fields[alias]; ok {
			return raw, alias, true
		}
	}
	return nil, "", false
}

func stringField(fields map[string]json.RawMessage, aliases ...string) (string, error) {
	raw, alias, ok := rawField(fields, aliases...)
	if !ok || string(raw) == "null" {
		return "", nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("goal %s: %w", alias, err)
	}
	return strings.TrimSpace(value), nil
}

func intField(fields map[string]json.RawMessage, aliases ...string) (int64, error) {
	raw, alias, ok := rawField(fields, aliases...)
	if !ok || string(raw) == "null" {
		return 0, nil
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("goal %s: %w", alias, err)
	}
	return value, nil
}

func intPtrField(fields map[string]json.RawMessage, aliases ...string) (*int64, error) {
	raw, alias, ok := rawField(fields, aliases...)
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("goal %s: %w", alias, err)
	}
	return &value, nil
}

func floatPtrField(fields map[string]json.RawMessage, aliases ...string) (*float64, error) {
	raw, alias, ok := rawField(fields, aliases...)
	if !ok || string(raw) == "null" {
		return nil, nil
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, fmt.Errorf("goal %s: %w", alias, err)
	}
	return &value, nil
}

func boolField(fields map[string]json.RawMessage, aliases ...string) (bool, error) {
	raw, alias, ok := rawField(fields, aliases...)
	if !ok || string(raw) == "null" {
		return false, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, fmt.Errorf("goal %s: %w", alias, err)
	}
	return value, nil
}

func unixSecondsField(fields map[string]json.RawMessage, aliases ...string) (time.Time, error) {
	seconds, err := intField(fields, aliases...)
	if err != nil || seconds <= 0 {
		return time.Time{}, err
	}
	return time.Unix(seconds, 0).UTC(), nil
}
