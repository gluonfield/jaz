package sqlite

import (
	"fmt"
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestLoadSessionOverviewIgnoresTranscriptWindowAndIncludesArchivedChildren(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	parent, err := store.CreateSession(storage.CreateSession{Slug: "overview-parent"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateSession(storage.CreateSession{Slug: "archived-child", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSession(storage.CreateSession{
		Slug: "sourced-child", ParentID: parent.ID, SourceType: storage.SourceLoopRun,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetArchived(child.ID, true); err != nil {
		t.Fatal(err)
	}
	events := []sessionevents.Event{
		providerSubagentEvent("/root/newton", "Newton"),
		providerSubagentEvent("/root/noether", "Noether"),
	}
	for i := range 300 {
		events = append(events, sessionevents.Event{Type: "note", Content: fmt.Sprintf("filler-%d", i)})
	}
	if err := store.AppendSessionEvents(parent.ID, events...); err != nil {
		t.Fatal(err)
	}

	view, err := store.LoadSessionOverview(t.Context(), parent.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Threads) != 1 || view.Threads[0].ID != child.ID || !view.Threads[0].Archived {
		t.Fatalf("threads = %#v", view.Threads)
	}
	if len(view.SubagentEvents) != 2 {
		t.Fatalf("subagent events = %#v", view.SubagentEvents)
	}
}

func providerSubagentEvent(id, name string) sessionevents.Event {
	return sessionevents.Event{
		Type: sessionevents.TypeProviderSubagent,
		ProviderSubagent: &sessionevents.ProviderSubagentEvent{
			Provider: "codex", ID: id, Name: name, Status: "completed",
		},
	}
}
