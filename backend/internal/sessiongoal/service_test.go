package sessiongoal

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestRefreshActiveCountsGoalTokensFromUsageEvents(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "goal", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	budget := int64(1000)
	service := New(store, sessionevents.New())
	state, err := service.Create(context.Background(), session.ID, CreateInput{
		Objective:   "ship goal accounting",
		TokenBudget: &budget,
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.TokensUsed != 0 {
		t.Fatalf("tokens_used after create = %d, want 0", state.TokensUsed)
	}
	if err := store.AddUsage(session.ID, storage.Usage{InputTokens: 100, OutputTokens: 25}); err != nil {
		t.Fatal(err)
	}
	state, err = service.RefreshActive(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.TokensUsed != 125 || state.RemainingTokens == nil || *state.RemainingTokens != 875 {
		t.Fatalf("goal tokens = %#v, want 125 used / 875 remaining", state)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal == nil || loaded.Goal.TokensUsed != 125 {
		t.Fatalf("stored goal = %#v", loaded.Goal)
	}
}

func TestCreateKeepsExistingActiveGoal(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "goal", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	service := New(store, sessionevents.New())
	first, err := service.Create(context.Background(), session.ID, CreateInput{Objective: "first objective"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Create(context.Background(), session.ID, CreateInput{Objective: "second objective"})
	if err != nil {
		t.Fatal(err)
	}
	if second.ID != first.ID || second.Objective != "first objective" {
		t.Fatalf("second create = %#v, want existing %#v", second, first)
	}
}

func TestGetWithoutActiveGoalReturnsEmptyGoal(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "goal", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	state, err := New(store, sessionevents.New()).Get(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state != nil {
		t.Fatalf("goal = %#v, want nil", state)
	}
}

func TestCompletedGoalCannotBeReopened(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "goal", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	service := New(store, sessionevents.New())
	if _, err := service.Create(context.Background(), session.ID, CreateInput{Objective: "finish"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), session.ID, UpdateInput{Status: "complete"}); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Update(context.Background(), session.ID, UpdateInput{Status: "active"}); !errors.Is(err, ErrNoActiveGoal) {
		t.Fatalf("reopen error = %v, want %v", err, ErrNoActiveGoal)
	}
}

func TestCompletedGoalCanReceiveFinalTurnUsageButIsNotActive(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "goal", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Minute).UTC()
	service := New(store, sessionevents.New())
	service.Now = func() time.Time { return base }
	first, err := service.Create(context.Background(), session.ID, CreateInput{Objective: "first objective"})
	if err != nil {
		t.Fatal(err)
	}
	service.Now = func() time.Time { return base.Add(10 * time.Second) }
	if _, err := service.Update(context.Background(), session.ID, UpdateInput{Status: "complete"}); err != nil {
		t.Fatal(err)
	}
	if state, err := service.Get(context.Background(), session.ID); err != nil || state != nil {
		t.Fatalf("get completed goal = %#v, %v; want nil, nil", state, err)
	}
	if err := store.AddUsage(session.ID, storage.Usage{InputTokens: 40, OutputTokens: 20}); err != nil {
		t.Fatal(err)
	}
	if refreshed, err := service.RefreshActive(context.Background(), session.ID); err != nil || refreshed != nil {
		t.Fatalf("refresh active completed = %#v, %v", refreshed, err)
	}
	refreshed, err := service.RefreshCurrentTurnSince(context.Background(), session.ID, base)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed == nil || refreshed.ID != first.ID || refreshed.TokensUsed != 60 {
		t.Fatalf("completed goal after final usage = %#v", refreshed)
	}
	service.Now = func() time.Time { return time.Now().Add(time.Second).UTC() }
	next, err := service.Create(context.Background(), session.ID, CreateInput{Objective: "second objective"})
	if err != nil {
		t.Fatal(err)
	}
	if next.ID == first.ID || next.Objective != "second objective" || next.TokensUsed != 0 {
		t.Fatalf("new goal after completed = %#v, first = %#v", next, first)
	}
}

func TestCompletedGoalBeforeTurnDoesNotReceiveCurrentTurnUsage(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "goal", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().Add(-time.Minute).UTC()
	service := New(store, sessionevents.New())
	service.Now = func() time.Time { return base }
	if _, err := service.Create(context.Background(), session.ID, CreateInput{Objective: "old objective"}); err != nil {
		t.Fatal(err)
	}
	service.Now = func() time.Time { return base.Add(10 * time.Second) }
	if _, err := service.Update(context.Background(), session.ID, UpdateInput{Status: "complete"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsage(session.ID, storage.Usage{InputTokens: 40, OutputTokens: 20}); err != nil {
		t.Fatal(err)
	}
	refreshed, err := service.RefreshCurrentTurnSince(context.Background(), session.ID, base.Add(20*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if refreshed != nil {
		t.Fatalf("refreshed old completed goal = %#v, want nil", refreshed)
	}
}
