package sqlite

import (
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionEventsPersistAndMirror(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "events"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: "acp_message", Content: "working"},
		sessionevents.Event{
			Type: "acp_tool",
			ACP: &sessionevents.ACPEvent{
				ID:        session.ID,
				ToolCalls: []sessionevents.ACPToolCall{{ID: "tool-1", Title: "Read file"}},
			},
		},
		sessionevents.Event{
			Type: "plan_update",
			Plan: &sessionevents.PlanEvent{
				Explanation: "Updated checklist",
				Plan:        []sessionevents.PlanEntry{{Content: "Run tests", Status: "pending"}},
			},
		},
	); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 3 || loaded[0].Seq != 1 || loaded[1].Seq != 2 || loaded[2].Seq != 3 {
		t.Fatalf("loaded events = %#v", loaded)
	}
	if loaded[1].ACP == nil || loaded[1].ACP.ToolCalls[0].Title != "Read file" {
		t.Fatalf("tool event = %#v", loaded[1])
	}
	if loaded[2].Plan == nil || loaded[2].Plan.Plan[0].Content != "Run tests" {
		t.Fatalf("plan event = %#v", loaded[2])
	}
	legacy, err := jsonstore.New(store.RootDir())
	if err != nil {
		t.Fatal(err)
	}
	mirrored, err := legacy.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mirrored) != 3 || mirrored[0].Content != "working" || mirrored[2].Plan == nil {
		t.Fatalf("mirrored events = %#v", mirrored)
	}
}

func TestCompactSessionEventsMergesStoredTextChunks(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "compact-events"})
	if err != nil {
		t.Fatal(err)
	}
	acpState := func(state string) *sessionevents.ACPEvent {
		return &sessionevents.ACPEvent{ID: session.ID, State: state}
	}
	thought := func(text string) *sessionevents.ACPEvent {
		acp := acpState("running")
		acp.Thought = text
		return acp
	}

	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: "acp_message", Content: "Hel", ACP: acpState("running")},
		sessionevents.Event{Type: "acp_message", Content: "lo", ACP: acpState("running")},
		sessionevents.Event{
			Type: "acp_tool",
			ACP: &sessionevents.ACPEvent{
				ID:        session.ID,
				ToolCalls: []sessionevents.ACPToolCall{{ID: "tool-1", Title: "Read", Status: "pending"}},
			},
		},
		sessionevents.Event{
			Type: "acp_tool",
			ACP: &sessionevents.ACPEvent{
				ID:        session.ID,
				ToolCalls: []sessionevents.ACPToolCall{{ID: "tool-1", Title: "Read", Status: "completed"}},
			},
		},
		sessionevents.Event{Type: "acp_thought", ACP: thought("Rea")},
		sessionevents.Event{Type: "acp_thought", ACP: thought("son")},
		sessionevents.Event{Type: "acp_message", Content: "Done.", ACP: acpState("running")},
		sessionevents.Event{Type: "acp_message", Content: "Next", ACP: acpState("idle")},
	); err != nil {
		t.Fatal(err)
	}

	removed, err := store.CompactSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 3 {
		t.Fatalf("removed = %d, want 3", removed)
	}
	loaded, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 5 {
		t.Fatalf("events = %d, want 5: %#v", len(loaded), loaded)
	}
	if loaded[0].Seq != 2 || loaded[0].Content != "Hello" {
		t.Fatalf("merged message = %#v", loaded[0])
	}
	if loaded[1].Seq != 3 || loaded[2].Seq != 4 {
		t.Fatalf("tool events should remain separate: %#v", loaded[1:3])
	}
	if loaded[3].Seq != 6 || loaded[3].ACP == nil || loaded[3].ACP.Thought != "Reason" {
		t.Fatalf("merged thought = %#v", loaded[3])
	}
	if loaded[4].Seq != 8 || loaded[4].Content != "Done.Next" || loaded[4].ACP.State != "idle" {
		t.Fatalf("final merged message = %#v", loaded[4])
	}

	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Type: "acp_message", Content: "again", ACP: acpState("running")}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded[len(loaded)-1].Seq != 9 {
		t.Fatalf("next seq = %d, want 9", loaded[len(loaded)-1].Seq)
	}
}
