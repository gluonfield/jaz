package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	usagequeries "github.com/wins/jaz/backend/internal/storage/sqlite/generated/usage"
)

func TestUsageEventsWindow(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "usage-window", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2024, 3, 10, 0, 0, 0, 0, time.UTC)
	q := usagequeries.New(store.db)
	for _, date := range []time.Time{start.Add(-time.Millisecond), start, start.Add(time.Millisecond), start.Add(2 * time.Millisecond), start.AddDate(1, 0, 0)} {
		if err := q.InsertUsageEvent(context.Background(), usagequeries.InsertUsageEventParams{
			ThreadID:    session.ID,
			WorkspaceID: storage.LocalWorkspaceID,
			Runtime:     storage.RuntimeACP,
			Source:      storage.UsageEventSourceTurn,
			InputTokens: 1,
			CreatedAtMs: date.UnixMilli(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, test := range []struct {
		name         string
		since, until time.Time
		want         []time.Time
	}{
		{"bounded", start, start.Add(2 * time.Millisecond), []time.Time{start, start.Add(time.Millisecond)}},
		{"sub-millisecond", start.Add(time.Nanosecond), start.Add(time.Millisecond + time.Nanosecond), []time.Time{start.Add(time.Millisecond)}},
		{"empty", start, start, nil},
		{"unbounded", start.Add(2 * time.Millisecond), time.Time{}, []time.Time{start.Add(2 * time.Millisecond), start.AddDate(1, 0, 0)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			events, err := store.UsageEvents(test.since, test.until)
			if err != nil || len(events) != len(test.want) {
				t.Fatalf("events = %#v, error = %v", events, err)
			}
			for i, want := range test.want {
				if !events[i].CreatedAt.Equal(want) {
					t.Fatalf("event %d at %s, want %s", i, events[i].CreatedAt, want)
				}
			}
		})
	}
}
