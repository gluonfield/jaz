package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	jsonstore "github.com/wins/jaz/backend/internal/storage/json"
)

func TestSessionEventsPersist(t *testing.T) {
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
		sessionevents.Event{
			Type: sessionevents.TypeSideChatMessage,
			SideChat: &sessionevents.SideChatEvent{
				ID:      "side-1",
				Role:    "assistant",
				Content: "side answer",
			},
		},
	); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 4 || loaded[0].Seq != 1 || loaded[1].Seq != 2 || loaded[2].Seq != 3 || loaded[3].Seq != 4 {
		t.Fatalf("loaded events = %#v", loaded)
	}
	after, err := store.LoadSessionEventsAfter(session.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != 3 || after[0].Seq != 2 || after[1].Seq != 3 || after[2].Seq != 4 {
		t.Fatalf("events after seq 1 = %#v", after)
	}
	if loaded[1].ACP == nil || loaded[1].ACP.ToolCalls[0].Title != "Read file" {
		t.Fatalf("tool event = %#v", loaded[1])
	}
	if loaded[1].ProjectionKey == "" || loaded[1].ProjectionOp != sessionevents.ProjectionReplace {
		t.Fatalf("tool projection = %#v", loaded[1])
	}
	if loaded[2].Plan == nil || loaded[2].Plan.Plan[0].Content != "Run tests" {
		t.Fatalf("plan event = %#v", loaded[2])
	}
	if loaded[3].SideChat == nil || loaded[3].SideChat.Content != "side answer" {
		t.Fatalf("side chat event = %#v", loaded[3])
	}
}

func TestLoadLatestACPTurnUsesBoundedDurableBoundary(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "latest-turn"})
	if err != nil {
		t.Fatal(err)
	}
	terminal := func() sessionevents.Event {
		return sessionevents.Event{Type: "acp", ACP: &sessionevents.ACPEvent{ID: session.ID, State: "idle"}}
	}
	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: "note", Content: "turn-1"}, terminal(),
		sessionevents.Event{Type: "note", Content: "turn-2"}, terminal(),
		sessionevents.Event{Type: "note", Content: "turn-3"},
	); err != nil {
		t.Fatal(err)
	}
	events, err := store.LoadLatestACPTurn(t.Context(), session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Content != "turn-2" || events[2].Content != "turn-3" {
		t.Fatalf("latest turn events = %#v", events)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := store.LoadLatestACPTurn(ctx, session.ID); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled load error = %v", err)
	}
}

func TestSessionEventsIgnoreStaleJSONMirror(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "sqlite-events-only"})
	if err != nil {
		t.Fatal(err)
	}
	mirror, err := jsonstore.New(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := mirror.AppendSessionEvents(session.ID, sessionevents.Event{Type: "acp_message", Content: "stale"}); err != nil {
		t.Fatal(err)
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("SQLite resurrected stale mirror events: %#v", events)
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

func TestSessionEventsCoalesceSnapshotsAndPreserveTextDeltas(t *testing.T) {
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
	acpTextRun := func(state, textRunID string) *sessionevents.ACPEvent {
		acp := acpState(state)
		acp.TextRunID = textRunID
		return acp
	}
	thought := func(text, textRunID string) *sessionevents.ACPEvent {
		acp := acpState("running")
		acp.Thought = text
		acp.TextRunID = textRunID
		return acp
	}

	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: "acp_message", Content: "Hel", ACP: acpTextRun("running", "message:m1")},
		sessionevents.Event{Type: "acp_message", Content: "lo", ACP: acpTextRun("running", "message:m1")},
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
		sessionevents.Event{Type: "acp_thought", ACP: thought("Rea", "message:t1")},
		sessionevents.Event{Type: "acp_thought", ACP: thought("son", "message:t1")},
		sessionevents.Event{Type: "acp_message", Content: "Done.", ACP: acpState("running")},
		sessionevents.Event{Type: "acp", ACP: acpState("running")},
		sessionevents.Event{Type: "acp_message", Content: "Next", ACP: acpState("idle")},
	); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 8 || stored[0].Seq != 1 || stored[0].Content != "Hel" || stored[1].Seq != 2 || stored[1].Content != "lo" || stored[2].Seq != 4 || stored[2].ACP.ToolCalls[0].Status != "completed" {
		t.Fatalf("stored events = %#v", stored)
	}

	loaded := sessionevents.CompactTranscript(stored)
	if len(loaded) != 6 {
		t.Fatalf("events = %d, want 6: %#v", len(loaded), loaded)
	}
	if loaded[0].Seq != 2 || loaded[0].Content != "Hello" {
		t.Fatalf("merged message = %#v", loaded[0])
	}
	if loaded[1].Seq != 4 || loaded[1].ACP == nil || loaded[1].ACP.ToolCalls[0].Status != "completed" {
		t.Fatalf("tool snapshot = %#v", loaded[1])
	}
	if loaded[2].Seq != 6 || loaded[2].ACP == nil || loaded[2].ACP.Thought != "Reason" {
		t.Fatalf("merged thought = %#v", loaded[2])
	}
	if loaded[3].Seq != 7 || loaded[3].Content != "Done." {
		t.Fatalf("message before running status = %#v", loaded[3])
	}
	if loaded[4].Seq != 8 || loaded[4].Type != "acp" {
		t.Fatalf("hidden status should remain at seq 8: %#v", loaded[4])
	}
	if loaded[5].Seq != 9 || loaded[5].Content != "Next" || loaded[5].ACP.State != "idle" {
		t.Fatalf("message after running status = %#v", loaded[5])
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

func TestSessionEventsDurablyCompleteSparseProviderSubagentUpdates(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "provider-updates"})
	if err != nil {
		t.Fatal(err)
	}
	key := sessionevents.ProviderSubagentProjectionKey(session.ID, sessionevents.ProviderSubagentEvent{Provider: "codex", ID: "worker"})
	event := func(status string, complete bool) sessionevents.Event {
		subagent := &sessionevents.ProviderSubagentEvent{Provider: "codex", ID: "worker", Status: status}
		if complete {
			subagent.Name = "Newton"
			subagent.Task = "audit"
		}
		return sessionevents.Event{
			Type:             sessionevents.TypeProviderSubagent,
			ProjectionKey:    key,
			ProviderSubagent: subagent,
		}
	}
	if err := store.AppendSessionEvents(session.ID, event("running", true)); err != nil {
		t.Fatal(err)
	}
	// A separate append models a manager restart with no volatile projection state.
	if err := store.AppendSessionEvents(session.ID, event("completed", false)); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].Seq != 2 || stored[0].ProviderSubagent == nil || stored[0].ProviderSubagent.Name != "Newton" || stored[0].ProviderSubagent.Status != "completed" {
		t.Fatalf("stored provider projection = %#v", stored)
	}
}

func TestSessionEventsPreserveTextBoundariesAndToolContinuations(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "text-boundaries"})
	if err != nil {
		t.Fatal(err)
	}
	text := func(content string) sessionevents.Event {
		return sessionevents.Event{
			Type:    sessionevents.TypeACPMessage,
			Content: content,
			ACP:     &sessionevents.ACPEvent{ID: session.ID, State: "running", TextRunID: "message:m1"},
		}
	}
	plan := &sessionevents.ACPEvent{
		ID:    session.ID,
		State: "running",
		Plan:  []sessionevents.PlanEntry{{Content: "Inspect files", Status: "completed"}},
	}
	tool := &sessionevents.ACPEvent{
		ID:        session.ID,
		State:     "running",
		ToolCalls: []sessionevents.ACPToolCall{{ID: "tool-1", Title: "Read", Status: "completed"}},
	}
	if err := store.AppendSessionEvents(session.ID,
		text("before"),
		sessionevents.Event{Type: "acp", ACP: plan},
		text("after"),
		text("!"),
		sessionevents.Event{Type: "acp_tool", ACP: tool},
		text(" tool"),
	); err != nil {
		t.Fatal(err)
	}

	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 6 {
		t.Fatalf("stored events = %#v", events)
	}
	projected := sessionevents.CompactTranscript(events)
	if len(projected) != 4 || projected[0].Content != "before" || projected[1].ACP.Plan == nil || projected[2].Type != "acp_tool" || projected[3].Content != "after! tool" {
		t.Fatalf("projected events = %#v", projected)
	}
}

func TestSessionEventsRetainTextBoundaryWhenPlansUpdate(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "plan-boundary"})
	if err != nil {
		t.Fatal(err)
	}
	text := func(content string) sessionevents.Event {
		return sessionevents.Event{
			Type:    sessionevents.TypeACPMessage,
			Content: content,
			ACP:     &sessionevents.ACPEvent{ID: session.ID, State: "running", TextRunID: "message:m1"},
		}
	}
	plan := func(status string) sessionevents.Event {
		return sessionevents.Event{Type: "acp", ACP: &sessionevents.ACPEvent{
			ID: session.ID, State: "running", Plan: []sessionevents.PlanEntry{{Content: "Inspect", Status: status}},
		}}
	}
	if err := store.AppendSessionEvents(session.ID, text("before"), plan("pending"), text("after"), plan("completed")); err != nil {
		t.Fatal(err)
	}

	raw, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 4 {
		t.Fatalf("stored events lost an ordering boundary: %#v", raw)
	}
	projected := sessionevents.CompactTranscript(raw)
	var messages []string
	for _, event := range projected {
		if event.Type == sessionevents.TypeACPMessage {
			messages = append(messages, event.Content)
		}
	}
	if len(messages) != 2 || messages[0] != "before" || messages[1] != "after" {
		t.Fatalf("text crossed a retained plan boundary: %#v", projected)
	}
}

func TestSessionEventReplayPreservesTextDelta(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "text-replay"})
	if err != nil {
		t.Fatal(err)
	}
	text := func(content string) sessionevents.Event {
		return sessionevents.Event{
			Type:    sessionevents.TypeACPMessage,
			Content: content,
			ACP:     &sessionevents.ACPEvent{ID: session.ID, State: "running", TextRunID: "message:m1"},
		}
	}
	if err := store.AppendSessionEvents(session.ID, text("Hel")); err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	after := snapshot[len(snapshot)-1].Seq
	if err := store.AppendSessionEvents(session.ID, text("lo")); err != nil {
		t.Fatal(err)
	}
	replay, err := store.LoadSessionEventsAfter(session.ID, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(replay) != 1 || replay[0].Content != "lo" {
		t.Fatalf("replay = %#v, want one append-only delta", replay)
	}
	projected := sessionevents.CompactTranscript(append(snapshot, replay...))
	if len(projected) != 1 || projected[0].Content != "Hello" {
		t.Fatalf("replayed transcript = %#v", projected)
	}
}

func TestSessionEventTextAppendDoesNotDecodeInterveningPayloads(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "text-stream-state"})
	if err != nil {
		t.Fatal(err)
	}
	text := func(content string) sessionevents.Event {
		return sessionevents.Event{
			Type:    sessionevents.TypeACPMessage,
			Content: content,
			ACP:     &sessionevents.ACPEvent{ID: session.ID, State: "running", TextRunID: "message:m1"},
		}
	}
	tool := sessionevents.Event{
		Type: "acp_tool",
		ACP: &sessionevents.ACPEvent{
			ID:        session.ID,
			State:     "running",
			ToolCalls: []sessionevents.ACPToolCall{{ID: "tool-1", Status: "completed"}},
		},
	}
	if err := store.AppendSessionEvents(session.ID, text("Hel"), tool); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE session_events SET acp = '{' WHERE thread_id = ? AND type = 'acp_tool'`, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, text("lo")); err != nil {
		t.Fatalf("text append decoded an intervening event: %v", err)
	}
	var content string
	if err := store.db.QueryRow(`SELECT content FROM session_events WHERE thread_id = ? AND seq = 3`, session.ID).Scan(&content); err != nil {
		t.Fatal(err)
	}
	if content != "lo" {
		t.Fatalf("content = %q, want lo", content)
	}
}

func TestTextAppendDoesNotDecodePreviousPayload(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "text-append-cost"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO session_events (thread_id, seq, type, content, acp, created_at_ms, coalesce_key)
		VALUES (?, 1, 'acp_message', 'old', '{', 1, '')`, session.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type:    sessionevents.TypeACPMessage,
		Content: "new",
		ACP:     &sessionevents.ACPEvent{ID: session.ID, TextRunID: "message:m1"},
	}); err != nil {
		t.Fatalf("append decoded an unrelated previous payload: %v", err)
	}
	events, err := store.LoadSessionEventsAfter(session.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Seq != 2 || events[0].Content != "new" {
		t.Fatalf("new delta = %#v", events)
	}
}

func TestSessionEventsProjectSparseProviderSubagentUpdates(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "provider-subagent"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: sessionevents.TypeProviderSubagent, ProviderSubagent: &sessionevents.ProviderSubagentEvent{
			Provider: "codex", ID: "worker-1", Name: "worker", Prompt: "inspect", Status: "running",
		}},
		sessionevents.Event{Type: sessionevents.TypeProviderSubagent, ProviderSubagent: &sessionevents.ProviderSubagentEvent{
			Provider: "codex", ID: "worker-1", Status: "completed",
		}},
	); err != nil {
		t.Fatal(err)
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 2 {
		t.Fatalf("stored provider updates = %#v", events)
	}
	projected := sessionevents.CompactTranscript(events)
	if len(projected) != 1 || projected[0].Seq != 2 || projected[0].ProviderSubagent == nil ||
		projected[0].ProviderSubagent.Status != "completed" || projected[0].ProviderSubagent.Name != "worker" ||
		projected[0].ProviderSubagent.Prompt != "inspect" {
		t.Fatalf("provider subagent = %#v", projected)
	}
}
