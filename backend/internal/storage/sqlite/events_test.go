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
	after, err := store.LoadSessionEventsAfter(session.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 2 || after[0].Seq != 2 || after[1].Seq != 3 {
		t.Fatalf("events after seq 1 = %#v", after)
	}
	if loaded[1].ACP == nil || loaded[1].ACP.ToolCalls[0].Title != "Read file" {
		t.Fatalf("tool event = %#v", loaded[1])
	}
	if loaded[2].Plan == nil || loaded[2].Plan.Plan[0].Content != "Run tests" {
		t.Fatalf("plan event = %#v", loaded[2])
	}
	mirror, err := jsonstore.New(store.RootDir())
	if err != nil {
		t.Fatal(err)
	}
	mirrored, err := mirror.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(mirrored) != 3 || mirrored[0].Content != "working" || mirrored[2].Plan == nil {
		t.Fatalf("mirrored events = %#v", mirrored)
	}
}

func TestSessionEventsLoadAfterSeqFallsBackToMirror(t *testing.T) {
	root := t.TempDir()
	mirror, err := jsonstore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	session, err := mirror.CreateSession(storage.CreateSession{Slug: "mirror-events"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mirror.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: "acp_message", Content: "one"},
		sessionevents.Event{Type: "acp_message", Content: "two"},
	); err != nil {
		t.Fatal(err)
	}

	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	events, err := store.LoadSessionEventsAfter(session.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Seq != 2 || events[0].Content != "two" {
		t.Fatalf("mirrored events after seq 1 = %#v", events)
	}
}

func TestLoopCreatedEventRoundTripsThroughContentColumn(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "loop-card"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: sessionevents.TypeLoopCreated,
		LoopCreated: &sessionevents.LoopCreatedEvent{
			LoopID:   "loop-1",
			LoopName: "Hourly politics briefing",
			Schedule: "13 * * * *",
			Timezone: "Europe/London",
			Agent:    "codex",
			Status:   "active",
			Boards:   []sessionevents.LoopBoardRef{{ID: "board-1", Name: "News"}},
		},
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 {
		t.Fatalf("loaded events = %#v", loaded)
	}
	lc := loaded[0].LoopCreated
	if lc == nil {
		t.Fatalf("loop_created payload was dropped: %#v", loaded[0])
	}
	if lc.LoopID != "loop-1" || lc.Schedule != "13 * * * *" || lc.Agent != "codex" {
		t.Fatalf("loop payload = %#v", lc)
	}
	if len(lc.Boards) != 1 || lc.Boards[0].ID != "board-1" || lc.Boards[0].Name != "News" {
		t.Fatalf("boards = %#v", lc.Boards)
	}
	if loaded[0].Content != "" {
		t.Fatalf("content should be collapsed into the typed payload, got %q", loaded[0].Content)
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
		sessionevents.Event{Type: "acp", ACP: acpState("running")},
		sessionevents.Event{Type: "acp_message", Content: "Next", ACP: acpState("idle")},
	); err != nil {
		t.Fatal(err)
	}

	removed, err := store.CompactSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Fatalf("removed = %d, want 2", removed)
	}
	loaded, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 7 {
		t.Fatalf("events = %d, want 7: %#v", len(loaded), loaded)
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
	if loaded[4].Seq != 7 || loaded[4].Content != "Done." {
		t.Fatalf("message before hidden status = %#v", loaded[4])
	}
	if loaded[5].Seq != 8 || loaded[5].Type != "acp" {
		t.Fatalf("hidden status should remain at seq 8: %#v", loaded[5])
	}
	if loaded[6].Seq != 9 || loaded[6].Content != "Next" || loaded[6].ACP.State != "idle" {
		t.Fatalf("message after hidden status = %#v", loaded[6])
	}

	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Type: "acp_message", Content: "again", ACP: acpState("running")}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded[len(loaded)-1].Seq != 10 {
		t.Fatalf("next seq = %d, want 10", loaded[len(loaded)-1].Seq)
	}
}
