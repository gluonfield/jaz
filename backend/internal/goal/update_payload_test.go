package goal

import (
	"encoding/json"
	"testing"
)

func TestUpdatePayloadAcceptsCodexSnakeCase(t *testing.T) {
	raw := []byte(`{
		"thread_id": "thread-1",
		"goal_id": "goal-1",
		"objective": "ship it",
		"status": "budget_limited",
		"token_budget": 100,
		"tokens_used": 120,
		"time_used_seconds": 9,
		"blocked_reason": "token budget reached",
		"completion_review": "not_achieved",
		"active_subagent_id": "worker-1"
	}`)
	var payload UpdatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	state := payload.State()
	normalized := NormalizeState(&state)
	if normalized == nil {
		t.Fatalf("state did not normalize: %#v", state)
	}
	if normalized.ThreadID != "thread-1" ||
		normalized.ProviderGoalID != "goal-1" ||
		normalized.Status != StatusBudgetLimited ||
		normalized.TokenBudget == nil ||
		*normalized.TokenBudget != 100 ||
		normalized.TokensUsed != 120 ||
		normalized.RemainingTokens == nil ||
		*normalized.RemainingTokens != 0 ||
		normalized.BlockedReason != "token budget reached" ||
		normalized.CompletionReview != "not_achieved" ||
		normalized.ActiveSubagentID != "worker-1" {
		t.Fatalf("normalized state = %#v", normalized)
	}
}

func TestPublicStateIsUIShape(t *testing.T) {
	budget := int64(100)
	state := State{
		Identity: Identity{ID: "goal-1", ThreadID: "thread-1", Objective: "ship it", Status: StatusActive},
		Budget:   Budget{TokenBudget: &budget, TokensUsed: 25},
		Progress: Progress{
			TimeUsedSeconds: 9,
			ProgressMessage: "running tests",
		},
	}
	normalized := NormalizeState(&state)
	if normalized == nil {
		t.Fatal("state did not normalize")
	}
	data, err := json.Marshal(PublicStateFrom(normalized))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	for _, nested := range []string{"Identity", "Budget", "Progress", "Review", "Cost", "Timestamps"} {
		if _, ok := fields[nested]; ok {
			t.Fatalf("state JSON contains nested %q: %s", nested, data)
		}
	}
	for _, field := range []string{"id", "thread_id", "objective", "status", "token_budget", "tokens_used", "remaining_tokens", "time_used_seconds"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("state JSON missing flat field %q: %s", field, data)
		}
	}
	for _, field := range []string{"budget_source", "progress_message", "created_at", "updated_at"} {
		if _, ok := fields[field]; ok {
			t.Fatalf("state JSON contains UI-hidden field %q: %s", field, data)
		}
	}
}

func TestStateStorageJSONKeepsInternalSnapshot(t *testing.T) {
	budget := int64(100)
	state := State{
		Identity: Identity{Objective: "ship it", Status: StatusActive},
		Budget:   Budget{TokenBudget: &budget, TokensUsed: 25},
		Progress: Progress{
			ProgressMessage: "running tests",
		},
	}
	normalized := NormalizeState(&state)
	if normalized == nil {
		t.Fatal("state did not normalize")
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		t.Fatal(err)
	}
	var roundTrip State
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatal(err)
	}
	roundTripState := NormalizeState(&roundTrip)
	if roundTripState == nil ||
		roundTripState.Objective != "ship it" ||
		roundTripState.TokenBudget == nil ||
		*roundTripState.TokenBudget != 100 ||
		roundTripState.ProgressMessage != "running tests" {
		t.Fatalf("round trip state = %#v", roundTripState)
	}
}

func TestNormalizeStateRejectsNegativeRemainingTokens(t *testing.T) {
	remaining := int64(-1)
	state := State{
		Identity: Identity{Objective: "ship it", Status: StatusActive},
		Budget:   Budget{RemainingTokens: &remaining},
	}
	if normalized := NormalizeState(&state); normalized != nil {
		t.Fatalf("normalized state = %#v, want nil", normalized)
	}
}

func TestUpdatePayloadAcceptsProviderRemainingTokens(t *testing.T) {
	raw := []byte(`{
		"objective": "ship it",
		"status": "active",
		"remaining_tokens": 42
	}`)
	var payload UpdatePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	state := payload.State()
	normalized := NormalizeState(&state)
	if normalized == nil || normalized.RemainingTokens == nil || *normalized.RemainingTokens != 42 {
		t.Fatalf("normalized state = %#v", normalized)
	}
}
