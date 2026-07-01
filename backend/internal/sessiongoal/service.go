package sessiongoal

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/wins/jaz/backend/internal/goal"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type Store interface {
	storage.SessionStore
	storage.SessionEventAppender
	storage.UsageEventStore
}

type EventPublisher interface {
	Publish(sessionevents.Event)
}

type Service struct {
	Store  Store
	Events EventPublisher
	Now    func() time.Time
}

type CreateInput struct {
	Objective   string `json:"objective" jsonschema:"concise goal objective"`
	TokenBudget *int64 `json:"token_budget,omitempty" jsonschema:"optional token budget for this goal"`
}

type UpdateInput struct {
	Status      string `json:"status" jsonschema:"active, blocked, complete"`
	TokenBudget *int64 `json:"token_budget,omitempty" jsonschema:"optional replacement token budget"`
}

type refreshScope struct {
	includeCompletedSince time.Time
}

var (
	ErrNoStore       = errors.New("goal store is not configured")
	ErrNoSession     = errors.New("goal tool must be called from a Jaz session")
	ErrNoActiveGoal  = errors.New("no active goal")
	ErrGoalObjective = errors.New("goal objective is required")
	ErrInvalidGoal   = errors.New("invalid goal")
	ErrInvalidStatus = errors.New("invalid goal status")
	ErrInvalidBudget = errors.New("token_budget must be non-negative")
)

func New(store Store, events EventPublisher) *Service {
	return &Service{Store: store, Events: events}
}

func (s *Service) Create(_ context.Context, sessionID string, input CreateInput) (*goal.State, error) {
	if s.Store == nil {
		return nil, ErrNoStore
	}
	if sessionID == "" {
		return nil, ErrNoSession
	}
	if input.TokenBudget != nil && *input.TokenBudget < 0 {
		return nil, ErrInvalidBudget
	}
	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return nil, err
	}
	if goal.Active(session.Goal) {
		return s.refreshAndPublishUsageChange(session, session.Goal, s.now(), refreshScope{})
	}
	objective := strings.TrimSpace(input.Objective)
	if objective == "" {
		return nil, ErrGoalObjective
	}
	now := s.now()
	state := &goal.State{
		Identity: goal.Identity{
			ID:        "goal-" + uuid.NewString(),
			ThreadID:  session.ID,
			Objective: objective,
			Status:    goal.StatusActive,
		},
		Budget: goal.Budget{
			TokenBudget: input.TokenBudget,
		},
		Timestamps: goal.Timestamps{
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
	return s.publishRefreshed(session, state, now)
}

func (s *Service) Get(_ context.Context, sessionID string) (*goal.State, error) {
	if s.Store == nil {
		return nil, ErrNoStore
	}
	if sessionID == "" {
		return nil, ErrNoSession
	}
	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return nil, err
	}
	if !goal.Active(session.Goal) {
		return nil, nil
	}
	return s.refreshAndPublishUsageChange(session, session.Goal, s.now(), refreshScope{})
}

func (s *Service) Update(_ context.Context, sessionID string, input UpdateInput) (*goal.State, error) {
	if s.Store == nil {
		return nil, ErrNoStore
	}
	if sessionID == "" {
		return nil, ErrNoSession
	}
	if input.TokenBudget != nil && *input.TokenBudget < 0 {
		return nil, ErrInvalidBudget
	}
	status := goal.NormalizeStatus(strings.TrimSpace(input.Status))
	if status == "" || status == goal.StatusRequested || status == goal.StatusPaused || status == goal.StatusUsageLimited || status == goal.StatusBudgetLimited {
		return nil, ErrInvalidStatus
	}
	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return nil, err
	}
	if !goal.Active(session.Goal) {
		return nil, ErrNoActiveGoal
	}
	now := s.now()
	state := *session.Goal
	state.Status = status
	if input.TokenBudget != nil {
		state.TokenBudget = input.TokenBudget
	}
	state.UpdatedAt = now
	if status == goal.StatusComplete {
		state.CompletedAt = now
	}
	return s.publishRefreshed(session, &state, now)
}

func (s *Service) RefreshActive(ctx context.Context, sessionID string) (*goal.State, error) {
	return s.refreshCurrent(ctx, sessionID, refreshScope{})
}

func (s *Service) RefreshCurrentTurnSince(ctx context.Context, sessionID string, startedAt time.Time) (*goal.State, error) {
	return s.refreshCurrent(ctx, sessionID, refreshScope{includeCompletedSince: startedAt})
}

func (s *Service) refreshCurrent(_ context.Context, sessionID string, scope refreshScope) (*goal.State, error) {
	if s.Store == nil || sessionID == "" {
		return nil, nil
	}
	session, err := s.Store.LoadSession(sessionID)
	if err != nil {
		return nil, err
	}
	if !scope.refreshes(session.Goal) {
		return nil, nil
	}
	now := s.now()
	next, err := s.refresh(session, session.Goal, now, scope)
	if err != nil {
		return nil, err
	}
	return s.publishIfUsageChanged(session, session.Goal, next, now)
}

func (s *Service) publishRefreshed(session storage.Session, state *goal.State, now time.Time) (*goal.State, error) {
	refreshed, err := s.refresh(session, state, now, refreshScope{})
	if err != nil {
		return nil, err
	}
	return s.publish(session.ID, refreshed, now)
}

func (s *Service) refreshAndPublishUsageChange(session storage.Session, state *goal.State, now time.Time, scope refreshScope) (*goal.State, error) {
	next, err := s.refresh(session, state, now, scope)
	if err != nil {
		return nil, err
	}
	return s.publishIfUsageChanged(session, state, next, now)
}

func (s *Service) publishIfUsageChanged(session storage.Session, current *goal.State, next *goal.State, now time.Time) (*goal.State, error) {
	normalized := goal.NormalizeState(current)
	if normalized != nil && normalized.TokensUsed == next.TokensUsed && sameInt64(normalized.RemainingTokens, next.RemainingTokens) {
		return next, nil
	}
	return s.publish(session.ID, next, now)
}

func (s *Service) refresh(session storage.Session, state *goal.State, now time.Time, scope refreshScope) (*goal.State, error) {
	normalized := goal.NormalizeState(state)
	if normalized == nil || normalized.Objective == "" {
		return nil, ErrInvalidGoal
	}
	if normalized.ID == "" {
		normalized.ID = "goal-" + uuid.NewString()
	}
	normalized.ThreadID = session.ID
	if normalized.CreatedAt.IsZero() {
		normalized.CreatedAt = firstTime(session.CreatedAt, now)
	}
	if normalized.UpdatedAt.IsZero() {
		normalized.UpdatedAt = now
	}
	completed := normalized.Status == goal.StatusComplete && !normalized.CompletedAt.IsZero()
	if completed && !scope.countsCompletedThroughCurrentTurn() {
		normalized.TokensUsed = s.tokensBetween(session.ID, normalized.CreatedAt, normalized.CompletedAt)
	} else {
		normalized.TokensUsed = s.tokensBetween(session.ID, normalized.CreatedAt, time.Time{})
	}
	if completed {
		normalized.TimeUsedSeconds = nonNegativeSeconds(normalized.CompletedAt.Sub(normalized.CreatedAt))
	} else {
		normalized.TimeUsedSeconds = nonNegativeSeconds(now.Sub(normalized.CreatedAt))
	}
	normalized = goal.NormalizeState(normalized)
	if normalized == nil {
		return nil, ErrInvalidGoal
	}
	return normalized, nil
}

func (scope refreshScope) refreshes(state *goal.State) bool {
	if goal.Active(state) {
		return true
	}
	normalized := goal.NormalizeState(state)
	return normalized != nil &&
		normalized.Status == goal.StatusComplete &&
		!normalized.CompletedAt.IsZero() &&
		!scope.includeCompletedSince.IsZero() &&
		!normalized.CompletedAt.Before(scope.includeCompletedSince)
}

func (scope refreshScope) countsCompletedThroughCurrentTurn() bool {
	return !scope.includeCompletedSince.IsZero()
}

func (s *Service) publish(sessionID string, state *goal.State, now time.Time) (*goal.State, error) {
	events := []sessionevents.Event{{
		SessionID: sessionID,
		Type:      sessionevents.TypeGoalUpdate,
		Goal:      state,
		At:        now,
	}}
	if err := s.Store.AppendSessionEvents(sessionID, events...); err != nil {
		return nil, err
	}
	if s.Events != nil {
		s.Events.Publish(events[0])
	}
	return state, nil
}

func (s *Service) tokensBetween(sessionID string, start, end time.Time) int64 {
	events, err := s.Store.UsageEventsSince(start)
	if err != nil {
		return 0
	}
	var total int64
	for _, event := range events {
		if event.SessionID != sessionID {
			continue
		}
		if !end.IsZero() && event.CreatedAt.After(end) {
			continue
		}
		if event.Usage.TotalTokens > 0 {
			total += event.Usage.TotalTokens
			continue
		}
		total += event.Usage.ComponentTotal()
	}
	return total
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func firstTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value.UTC()
		}
	}
	return time.Now().UTC()
}

func nonNegativeSeconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	return int64(duration.Seconds())
}

func sameInt64(a, b *int64) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return *a == *b
	}
}
