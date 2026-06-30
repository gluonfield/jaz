package goal

import (
	"strings"
	"time"
)

type Status string
type BudgetSource string
type Source string

const (
	StatusRequested     Status = "requested"
	StatusActive        Status = "active"
	StatusPaused        Status = "paused"
	StatusBlocked       Status = "blocked"
	StatusUsageLimited  Status = "usageLimited"
	StatusBudgetLimited Status = "budgetLimited"
	StatusComplete      Status = "complete"
)

const (
	BudgetSourceGoal    BudgetSource = "goal"
	BudgetSourceSession BudgetSource = "session"
	BudgetSourceContext BudgetSource = "context"
	BudgetSourceCost    BudgetSource = "cost"
)

const SourceProvider Source = "provider"

type State struct {
	Identity
	Budget
	Progress
	Review
	Cost
	Timestamps
}

type Identity struct {
	Source         Source `json:"source,omitempty"`
	ThreadID       string `json:"thread_id,omitempty"`
	Provider       string `json:"provider,omitempty"`
	ProviderGoalID string `json:"provider_goal_id,omitempty"`
	Objective      string `json:"objective,omitempty"`
	Status         Status `json:"status"`
}

type Budget struct {
	BudgetSource    BudgetSource `json:"budget_source,omitempty"`
	TokenBudget     *int64       `json:"token_budget,omitempty"`
	TokensUsed      int64        `json:"tokens_used,omitempty"`
	RemainingTokens *int64       `json:"remaining_tokens,omitempty"`
}

type Progress struct {
	TimeUsedSeconds  int64  `json:"time_used_seconds,omitempty"`
	TurnCount        int64  `json:"turn_count,omitempty"`
	EvaluatedTurns   int64  `json:"evaluated_turns,omitempty"`
	AttemptCount     int64  `json:"attempt_count,omitempty"`
	ProgressMessage  string `json:"progress_message,omitempty"`
	BlockedReason    string `json:"blocked_reason,omitempty"`
	ActiveSubagentID string `json:"active_subagent_id,omitempty"`
	ActiveOperation  string `json:"active_operation,omitempty"`
}

type Review struct {
	EvaluatorReason  string `json:"evaluator_reason,omitempty"`
	CompletionReview string `json:"completion_review,omitempty"`
}

type Cost struct {
	CostUsedUSD   *float64 `json:"cost_used_usd,omitempty"`
	CostBudgetUSD *float64 `json:"cost_budget_usd,omitempty"`
	CostEstimated bool     `json:"cost_estimated,omitempty"`
}

type Timestamps struct {
	CreatedAt   time.Time `json:"created_at,omitzero"`
	UpdatedAt   time.Time `json:"updated_at,omitzero"`
	CompletedAt time.Time `json:"completed_at,omitzero"`
}

func NormalizeStatus(status string) Status {
	switch Status(strings.TrimSpace(status)) {
	case StatusRequested:
		return StatusRequested
	case StatusActive:
		return StatusActive
	case StatusPaused:
		return StatusPaused
	case StatusBlocked:
		return StatusBlocked
	case StatusUsageLimited:
		return StatusUsageLimited
	case StatusBudgetLimited:
		return StatusBudgetLimited
	case StatusComplete:
		return StatusComplete
	case Status("usage_limited"):
		return StatusUsageLimited
	case Status("budget_limited"):
		return StatusBudgetLimited
	case Status("completed"):
		return StatusComplete
	case Status("running"):
		return StatusActive
	default:
		return ""
	}
}

func NormalizeBudgetSource(source string) BudgetSource {
	switch BudgetSource(strings.TrimSpace(source)) {
	case BudgetSourceGoal:
		return BudgetSourceGoal
	case BudgetSourceSession:
		return BudgetSourceSession
	case BudgetSourceContext:
		return BudgetSourceContext
	case BudgetSourceCost:
		return BudgetSourceCost
	default:
		return ""
	}
}

func NormalizeSource(source string) Source {
	switch Source(strings.TrimSpace(source)) {
	case SourceProvider:
		return SourceProvider
	default:
		return ""
	}
}

func NormalizeState(state *State) *State {
	if state == nil {
		return nil
	}
	out := *state
	out.Source = NormalizeSource(string(out.Source))
	out.Status = NormalizeStatus(string(out.Status))
	out.BudgetSource = NormalizeBudgetSource(string(out.BudgetSource))
	if out.Status == "" ||
		out.TokensUsed < 0 ||
		out.TimeUsedSeconds < 0 ||
		out.TurnCount < 0 ||
		out.EvaluatedTurns < 0 ||
		out.AttemptCount < 0 ||
		negativeInt(out.RemainingTokens) ||
		negativeFloat(out.CostUsedUSD) ||
		negativeFloat(out.CostBudgetUSD) {
		return nil
	}
	if out.TokenBudget != nil {
		budget := *out.TokenBudget
		if budget < 0 {
			return nil
		}
		if out.BudgetSource == "" {
			out.BudgetSource = BudgetSourceGoal
		}
		remaining := budget - out.TokensUsed
		if remaining < 0 {
			remaining = 0
		}
		out.RemainingTokens = &remaining
	}
	return &out
}

func CompleteSnapshot(state *State) bool {
	normalized := NormalizeState(state)
	return normalized != nil && normalized.Objective != ""
}

func Active(state *State) bool {
	normalized := NormalizeState(state)
	return normalized != nil && normalized.Status != StatusComplete
}

func ProviderSnapshot(state *State) bool {
	normalized := NormalizeState(state)
	if normalized == nil || normalized.Objective == "" {
		return false
	}
	if normalized.Source == SourceProvider {
		return true
	}
	return !legacyRequestOnlySnapshot(normalized)
}

func legacyRequestOnlySnapshot(state *State) bool {
	return state.Source == "" &&
		state.Status == StatusRequested &&
		state.ThreadID == "" &&
		state.ProviderGoalID == "" &&
		state.Budget == Budget{} &&
		state.Progress == Progress{} &&
		state.Review == Review{} &&
		state.Cost == Cost{} &&
		state.CompletedAt.IsZero()
}

func negativeFloat(value *float64) bool {
	return value != nil && *value < 0
}

func negativeInt(value *int64) bool {
	return value != nil && *value < 0
}
