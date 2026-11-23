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

type pausedGoalUsageStore struct {
	*sqlitestore.Store
	loaded  chan struct{}
	proceed chan struct{}
}

func (s pausedGoalUsageStore) UsageEvents(since, until time.Time) ([]storage.UsageEvent, error) {
	close(s.loaded)
	<-s.proceed
	return s.Store.UsageEvents(since, until)
}

func (s pausedGoalUsageStore) UsageEventsForWorkspace(workspaceID string, since, until time.Time) ([]storage.UsageEvent, error) {
	close(s.loaded)
	<-s.proceed
	return s.Store.UsageEventsForWorkspace(workspaceID, since, until)
}

func TestClearWinsOverInFlightGoalWrites(t *testing.T) {
	for _, operation := range []string{"create", "update", "usage"} {
		t.Run(operation, func(t *testing.T) {
			store, session := newGoalTestSession(t)
			if operation != "create" {
				if _, err := New(store, nil).Create(t.Context(), session.ID, CreateInput{Objective: "Finish the work"}); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.AddUsage(session.ID, storage.Usage{InputTokens: 50}); err != nil {
				t.Fatal(err)
			}
			paused := pausedGoalUsageStore{Store: store, loaded: make(chan struct{}), proceed: make(chan struct{})}
			service := New(paused, nil)
			result := make(chan error, 1)
			go func() {
				var err error
				switch operation {
				case "create":
					_, err = service.Create(t.Context(), session.ID, CreateInput{Objective: "Late goal"})
				case "update":
					_, err = service.Update(t.Context(), session.ID, UpdateInput{Status: "active"})
				case "usage":
					_, err = service.RefreshActive(t.Context(), session.ID)
				}
				result <- err
			}()
			select {
			case <-paused.loaded:
			case err := <-result:
				t.Fatalf("operation returned before reading usage: %v", err)
			}
			_, clearErr := store.UpdateSessionGoal(session.ID, nil, nil)
			close(paused.proceed)
			if clearErr != nil {
				t.Fatal(clearErr)
			}
			err := <-result
			if !errors.Is(err, storage.ErrGoalChanged) && !errors.Is(err, storage.ErrGoalNotRequested) {
				t.Fatalf("late %s = %v", operation, err)
			}
			loaded, err := store.LoadSession(session.ID)
			if err != nil || loaded.Goal != nil || loaded.Turn == nil || loaded.Turn.GoalRequested || loaded.Status != storage.StatusRunning {
				t.Fatalf("session after late write = %+v, %v", loaded, err)
			}
			if err := store.StartSessionTurn(session.ID, storage.Turn{GoalRequested: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := New(store, nil).Create(t.Context(), session.ID, CreateInput{Objective: "Explicitly requested again"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestRefreshActiveExcludesCachedInputFromGoalTokens(t *testing.T) {
	store, session := newGoalTestSession(t)
	budget := int64(10_000)
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
	if err := store.AddUsage(session.ID, storage.Usage{
		InputTokens:           200_264,
		CachedInputTokens:     196_352,
		OutputTokens:          372,
		ReasoningOutputTokens: 257,
		TotalTokens:           200_636,
	}); err != nil {
		t.Fatal(err)
	}
	state, err = service.RefreshActive(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.TokensUsed != 4_284 || state.RemainingTokens == nil || *state.RemainingTokens != 5_716 {
		t.Fatalf("goal tokens = %#v, want 4,284 used / 5,716 remaining", state)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Goal == nil || loaded.Goal.TokensUsed != 4_284 {
		t.Fatalf("stored goal = %#v", loaded.Goal)
	}
}

func TestGoalTokensInfersMissingInputFromTotal(t *testing.T) {
	usage := storage.Usage{TotalTokens: 1_000, CachedInputTokens: 400, OutputTokens: 100}
	if got := goalTokens(usage); got != 600 {
		t.Fatalf("goal tokens = %d, want 600", got)
	}
}

func TestCreateKeepsExistingActiveGoal(t *testing.T) {
	store, session := newGoalTestSession(t)
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
	store, session := newGoalTestSession(t)
	state, err := New(store, sessionevents.New()).Get(context.Background(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state != nil {
		t.Fatalf("goal = %#v, want nil", state)
	}
}

func TestCompletedGoalCannotBeReopened(t *testing.T) {
	store, session := newGoalTestSession(t)
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
	store, session := newGoalTestSession(t)
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
	store, session := newGoalTestSession(t)
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

func newGoalTestSession(t *testing.T) (*sqlitestore.Store, storage.Session) {
	t.Helper()
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		store.Close()
	})
	session, err := store.CreateSession(storage.CreateSession{Slug: "goal", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartSessionTurn(session.ID, storage.Turn{GoalRequested: true}); err != nil {
		t.Fatal(err)
	}
	return store, session
}
