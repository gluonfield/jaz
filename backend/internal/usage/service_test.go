package usage

import (
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

type fakeUsageEventStore struct {
	events []storage.UsageEvent
	since  time.Time
	err    error
}

func (s *fakeUsageEventStore) UsageEventsSince(since time.Time) ([]storage.UsageEvent, error) {
	s.since = since
	if s.err != nil {
		return nil, s.err
	}
	return s.events, nil
}

func TestDailyAggregatesUsageByLocalDay(t *testing.T) {
	loc := time.FixedZone("plus2", 2*60*60)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, loc)
	store := &fakeUsageEventStore{events: []storage.UsageEvent{
		{
			SessionID: "ignored",
			Usage:     storage.Usage{InputTokens: 100},
			Source:    storage.UsageEventSourceTurn,
			CreatedAt: time.Date(2026, 6, 14, 21, 59, 0, 0,
				time.UTC),
		},
		{
			SessionID: "session-1",
			Usage: storage.Usage{
				InputTokens:  10,
				OutputTokens: 2,
			},
			Source:    storage.UsageEventSourceTurn,
			CreatedAt: time.Date(2026, 6, 14, 22, 30, 0, 0, time.UTC),
		},
		{
			SessionID: "imported",
			Usage: storage.Usage{
				InputTokens:  1_000_000,
				OutputTokens: 1_000_000,
			},
			Source:    storage.UsageEventSourceSessionImport,
			CreatedAt: time.Date(2026, 6, 14, 22, 45, 0, 0, time.UTC),
		},
		{
			SessionID:     "session-1",
			Runtime:       storage.RuntimeACP,
			Agent:         "codex",
			ModelProvider: "openai",
			Model:         "gpt-5.4",
			Usage: storage.Usage{
				CachedInputTokens: 3,
				CachedWriteTokens: 4,
				OutputTokens:      5,
			},
			Source:    storage.UsageEventSourceTurn,
			CreatedAt: time.Date(2026, 6, 15, 21, 59, 0, 0, time.UTC),
		},
		{
			SessionID: "session-2",
			Usage: storage.Usage{
				InputTokens:           7,
				CachedInputTokens:     11,
				CachedWriteTokens:     13,
				OutputTokens:          17,
				ReasoningOutputTokens: 19,
			},
			Source:    storage.UsageEventSourceTurn,
			CreatedAt: time.Date(2026, 6, 15, 22, 15, 0, 0, time.UTC),
		},
	}}
	daily, err := (Service{
		store: store,
		now:   func() time.Time { return now },
	}).Daily(DailyQuery{Days: 2, Location: loc})
	if err != nil {
		t.Fatal(err)
	}
	wantSince := time.Date(2026, 6, 14, 22, 0, 0, 0, time.UTC)
	if !store.since.Equal(wantSince) {
		t.Fatalf("since = %s, want %s", store.since, wantSince)
	}
	if len(daily) != 2 {
		t.Fatalf("days = %d, want 2", len(daily))
	}
	if daily[0].Date != "2026-06-15" || daily[0].SessionCount != 1 {
		t.Fatalf("first bucket = %#v", daily[0])
	}
	if daily[0].Usage.InputTokens != 10 ||
		daily[0].Usage.CachedInputTokens != 3 ||
		daily[0].Usage.CachedWriteTokens != 4 ||
		daily[0].Usage.OutputTokens != 7 ||
		daily[0].Usage.InputOutputTokens() != 17 {
		t.Fatalf("first bucket usage = %#v", daily[0].Usage)
	}
	if len(daily[0].Models) != 1 {
		t.Fatalf("first bucket models = %#v", daily[0].Models)
	}
	model := daily[0].Models[0]
	if model.Agent != "codex" || model.ModelProvider != "openai" || model.Model != "gpt-5.4" ||
		model.Usage.CachedInputTokens != 3 || model.Usage.CachedWriteTokens != 4 ||
		model.Usage.OutputTokens != 5 || model.SessionCount != 1 {
		t.Fatalf("first bucket model = %#v", model)
	}
	if daily[1].Date != "2026-06-16" || daily[1].SessionCount != 1 {
		t.Fatalf("second bucket = %#v", daily[1])
	}
	if daily[1].Usage.InputTokens != 7 ||
		daily[1].Usage.CachedInputTokens != 11 ||
		daily[1].Usage.CachedWriteTokens != 13 ||
		daily[1].Usage.OutputTokens != 17 ||
		daily[1].Usage.ReasoningOutputTokens != 19 ||
		daily[1].Usage.InputOutputTokens() != 24 {
		t.Fatalf("second bucket usage = %#v", daily[1].Usage)
	}
}

func TestModelsAggregatesACPUsageByModel(t *testing.T) {
	loc := time.FixedZone("plus2", 2*60*60)
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, loc)
	store := &fakeUsageEventStore{events: []storage.UsageEvent{
		{
			SessionID:     "codex-1",
			Runtime:       storage.RuntimeACP,
			Agent:         "codex",
			ModelProvider: "openai",
			Model:         "gpt-5.4",
			Usage: storage.Usage{
				InputTokens:  10,
				OutputTokens: 5,
			},
			Source:    storage.UsageEventSourceTurn,
			CreatedAt: time.Date(2026, 6, 14, 22, 30, 0, 0, time.UTC),
		},
		{
			SessionID:     "codex-1",
			Runtime:       storage.RuntimeACP,
			Agent:         "codex",
			ModelProvider: "openai",
			Model:         "gpt-5.4",
			Usage: storage.Usage{
				CachedInputTokens: 3,
				OutputTokens:      7,
			},
			Source:    storage.UsageEventSourceTurn,
			CreatedAt: time.Date(2026, 6, 15, 22, 15, 0, 0, time.UTC),
		},
		{
			SessionID:     "claude-1",
			Runtime:       storage.RuntimeACP,
			Agent:         "claude",
			ModelProvider: "anthropic",
			Model:         "sonnet",
			Usage: storage.Usage{
				InputTokens:  50,
				OutputTokens: 30,
			},
			Source:    storage.UsageEventSourceTurn,
			CreatedAt: time.Date(2026, 6, 15, 22, 20, 0, 0, time.UTC),
		},
		{
			SessionID: "imported-large",
			Runtime:   storage.RuntimeACP,
			Model:     "ignored-model",
			Usage: storage.Usage{
				InputTokens:  1_000_000,
				OutputTokens: 1_000_000,
			},
			Source:    storage.UsageEventSourceSessionImport,
			CreatedAt: time.Date(2026, 6, 15, 22, 25, 0, 0, time.UTC),
		},
		{
			SessionID: "imported",
			Runtime:   storage.RuntimeACP,
			Agent:     "codex",
			Model:     "gpt-5.4",
			Usage: storage.Usage{
				InputTokens:  1_000_000,
				OutputTokens: 1_000_000,
			},
			Source:    storage.UsageEventSourceSessionImport,
			CreatedAt: time.Date(2026, 6, 15, 22, 30, 0, 0, time.UTC),
		},
	}}
	models, err := (Service{
		store: store,
		now:   func() time.Time { return now },
	}).Models(DailyQuery{Days: 2, Location: loc})
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 2 {
		t.Fatalf("models = %#v, want 2 groups", models)
	}
	if models[0].Agent != "claude" || models[0].ModelProvider != "anthropic" || models[0].Model != "sonnet" {
		t.Fatalf("first model = %#v", models[0])
	}
	if models[0].Usage.InputOutputTokens() != 80 || models[0].SessionCount != 1 {
		t.Fatalf("first model totals = %#v", models[0])
	}
	if models[1].Agent != "codex" || models[1].ModelProvider != "openai" || models[1].Model != "gpt-5.4" {
		t.Fatalf("second model = %#v", models[1])
	}
	if models[1].Usage.InputTokens != 10 ||
		models[1].Usage.CachedInputTokens != 3 ||
		models[1].Usage.OutputTokens != 12 ||
		models[1].Usage.InputOutputTokens() != 22 ||
		models[1].SessionCount != 1 {
		t.Fatalf("second model totals = %#v", models[1])
	}
}

func TestDailyDefaultsAndClampsDays(t *testing.T) {
	now := time.Date(2026, 2, 3, 10, 0, 0, 0, time.UTC)
	service := Service{
		store: &fakeUsageEventStore{},
		now:   func() time.Time { return now },
	}
	daily, err := service.Daily(DailyQuery{Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	if len(daily) != DefaultDailyDays || daily[0].Date != "2026-01-05" || daily[len(daily)-1].Date != "2026-02-03" {
		t.Fatalf("default daily range = %#v", daily)
	}
	daily, err = service.Daily(DailyQuery{Days: MaxDailyDays + 1, Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	if len(daily) != MaxDailyDays {
		t.Fatalf("clamped days = %d, want %d", len(daily), MaxDailyDays)
	}
}

func TestDailyValidationAndUnsupportedStore(t *testing.T) {
	_, err := NewService(nil).Daily(DailyQuery{Days: -1, Location: time.UTC})
	if !errors.Is(err, ErrInvalidDays) {
		t.Fatalf("negative days error = %v, want ErrInvalidDays", err)
	}
	_, err = NewService(nil).Daily(DailyQuery{Days: 1, Location: time.UTC})
	if !errors.Is(err, ErrUnsupported) {
		t.Fatalf("nil store error = %v, want ErrUnsupported", err)
	}
}
