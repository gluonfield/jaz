package goal

import (
	"encoding/json"
	"testing"
	"time"
)

func TestPublicStateIsUIShape(t *testing.T) {
	budget := int64(100)
	state := State{
		Identity:        Identity{ID: "goal-1", ThreadID: "thread-1", Objective: "ship it", Status: StatusActive},
		Budget:          Budget{TokenBudget: &budget, TokensUsed: 25},
		TimeUsedSeconds: 9,
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
	for _, nested := range []string{"Identity", "Budget", "Timestamps"} {
		if _, ok := fields[nested]; ok {
			t.Fatalf("state JSON contains nested %q: %s", nested, data)
		}
	}
	for _, field := range []string{"id", "thread_id", "objective", "status", "token_budget", "tokens_used", "remaining_tokens", "time_used_seconds"} {
		if _, ok := fields[field]; !ok {
			t.Fatalf("state JSON missing flat field %q: %s", field, data)
		}
	}
	for _, field := range []string{"created_at", "updated_at"} {
		if _, ok := fields[field]; ok {
			t.Fatalf("state JSON contains UI-hidden field %q: %s", field, data)
		}
	}
}

func TestStateStorageJSONKeepsInternalSnapshot(t *testing.T) {
	budget := int64(100)
	created := time.Date(2026, time.July, 2, 9, 0, 0, 0, time.UTC)
	state := State{
		Identity:   Identity{Objective: "ship it", Status: StatusActive},
		Budget:     Budget{TokenBudget: &budget, TokensUsed: 25},
		Timestamps: Timestamps{CreatedAt: created},
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
		!roundTripState.CreatedAt.Equal(created) {
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

func TestNormalizeStateMarksReachedBudgetAsLimited(t *testing.T) {
	budget := int64(100)
	state := State{
		Identity: Identity{Objective: "ship it", Status: StatusActive},
		Budget:   Budget{TokenBudget: &budget, TokensUsed: 100},
	}
	normalized := NormalizeState(&state)
	if normalized == nil || normalized.Status != StatusBudgetLimited || normalized.RemainingTokens == nil || *normalized.RemainingTokens != 0 {
		t.Fatalf("normalized state = %#v, want budget-limited with no remaining tokens", normalized)
	}
	if !Active(normalized) || Continuable(normalized) {
		t.Fatalf("budget-limited goal active = %t, continuable = %t", Active(normalized), Continuable(normalized))
	}
	raised := int64(200)
	normalized.Status = StatusActive
	normalized.TokenBudget = &raised
	normalized = NormalizeState(normalized)
	if normalized == nil || normalized.Status != StatusActive || !Continuable(normalized) {
		t.Fatalf("goal after raised budget = %#v, want active and continuable", normalized)
	}
}
