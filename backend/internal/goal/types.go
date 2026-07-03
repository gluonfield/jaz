package goal

import (
	"strings"
	"time"
)

type Status string

const (
	StatusRequested     Status = "requested"
	StatusActive        Status = "active"
	StatusPaused        Status = "paused"
	StatusBlocked       Status = "blocked"
	StatusUsageLimited  Status = "usageLimited"
	StatusBudgetLimited Status = "budgetLimited"
	StatusComplete      Status = "complete"
)

type State struct {
	Identity
	Budget
	Timestamps
	TimeUsedSeconds int64 `json:"time_used_seconds,omitempty"`
}

type Identity struct {
	ID        string `json:"id,omitempty"`
	ThreadID  string `json:"thread_id,omitempty"`
	Objective string `json:"objective,omitempty"`
	Status    Status `json:"status"`
}

type Budget struct {
	TokenBudget     *int64 `json:"token_budget,omitempty"`
	TokensUsed      int64  `json:"tokens_used,omitempty"`
	RemainingTokens *int64 `json:"remaining_tokens,omitempty"`
}

type Timestamps struct {
	CreatedAt   time.Time `json:"created_at,omitzero"`
	UpdatedAt   time.Time `json:"updated_at,omitzero"`
	CompletedAt time.Time `json:"completed_at,omitzero"`
}

type PublicState struct {
	ID              string `json:"id,omitempty"`
	ThreadID        string `json:"thread_id,omitempty"`
	Objective       string `json:"objective,omitempty"`
	Status          Status `json:"status"`
	TokenBudget     *int64 `json:"token_budget,omitempty"`
	TokensUsed      int64  `json:"tokens_used,omitempty"`
	RemainingTokens *int64 `json:"remaining_tokens,omitempty"`
	TimeUsedSeconds int64  `json:"time_used_seconds,omitempty"`
}

func PublicStateFrom(state *State) *PublicState {
	normalized := NormalizeState(state)
	if normalized == nil {
		return nil
	}
	return &PublicState{
		ID:              normalized.ID,
		ThreadID:        normalized.ThreadID,
		Objective:       normalized.Objective,
		Status:          normalized.Status,
		TokenBudget:     normalized.TokenBudget,
		TokensUsed:      normalized.TokensUsed,
		RemainingTokens: normalized.RemainingTokens,
		TimeUsedSeconds: normalized.TimeUsedSeconds,
	}
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

func NormalizeState(state *State) *State {
	if state == nil {
		return nil
	}
	out := *state
	out.Status = NormalizeStatus(string(out.Status))
	if out.Status == "" ||
		out.TokensUsed < 0 ||
		out.TimeUsedSeconds < 0 ||
		negativeInt(out.RemainingTokens) {
		return nil
	}
	if out.TokenBudget != nil {
		budget := *out.TokenBudget
		if budget < 0 {
			return nil
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
	return normalized != nil && normalized.Objective != "" && normalized.Status != StatusRequested
}

func Active(state *State) bool {
	normalized := NormalizeState(state)
	return normalized != nil &&
		normalized.Objective != "" &&
		normalized.Status != StatusRequested &&
		normalized.Status != StatusComplete
}

// Continuable reports whether an automatic goal continuation turn should run.
// Unlike Active, only StatusActive goals with budget headroom continue.
func Continuable(state *State) bool {
	normalized := NormalizeState(state)
	if normalized == nil || normalized.Objective == "" || normalized.Status != StatusActive {
		return false
	}
	if normalized.TokenBudget != nil && normalized.TokensUsed >= *normalized.TokenBudget {
		return false
	}
	return true
}

func negativeInt(value *int64) bool {
	return value != nil && *value < 0
}
