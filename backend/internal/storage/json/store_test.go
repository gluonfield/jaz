package jsonstore

import (
	stdjson "encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/provider"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	usagecore "github.com/wins/jaz/backend/internal/usage"
)

func TestSessionsHaveStableUniqueSlugsAndRootListing(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	first, err := store.CreateSession(storage.CreateSession{Slug: "Review ACP Transport", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateSession(storage.CreateSession{Slug: "review-acp-transport", ParentID: first.ID, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if first.Slug != "review-acp-transport" {
		t.Fatalf("unexpected first slug %q", first.Slug)
	}
	if second.Slug != "review-acp-transport-2" {
		t.Fatalf("unexpected second slug %q", second.Slug)
	}

	roots, err := store.ListSessions(storage.SessionFilter{RootOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(roots) != 1 || roots[0].ID != first.ID {
		t.Fatalf("unexpected roots %#v", roots)
	}

	resolved, err := store.LoadSession(second.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.ID != second.ID {
		t.Fatalf("resolved %s, want %s", resolved.ID, second.ID)
	}
}

// Slugs are assigned once at creation; saves persist them verbatim so the
// mirror never rescans session metadata to re-derive uniqueness.
func TestSaveSessionPersistsSlugVerbatim(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	session, err := store.CreateSession(storage.CreateSession{Slug: "alpha", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}

	for range 3 {
		if err := store.SaveSession(session); err != nil {
			t.Fatal(err)
		}
		session, err = store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	if session.Slug != "alpha" {
		t.Fatalf("slug drifted to %q after repeated saves", session.Slug)
	}

	session.Slug = "Custom Slug"
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}
	saved, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Slug != "Custom Slug" {
		t.Fatalf("slug = %q, want caller value persisted verbatim", saved.Slug)
	}
}

func TestDefaultSlugIgnoresACPAgent(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	session, err := store.CreateSession(storage.CreateSession{
		Runtime: storage.RuntimeACP,
		RuntimeRef: &storage.RuntimeRef{
			Type:  storage.RuntimeACP,
			Agent: "claude",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(session.Slug, "chat-") {
		t.Fatalf("slug = %q, want neutral chat fallback", session.Slug)
	}
}

func TestSessionQueuedMessagesRoundTrip(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "queued"})
	if err != nil {
		t.Fatal(err)
	}
	session.QueuedMessages = []storage.QueuedMessage{
		storage.NewQueuedMessage("one prompt", nil),
		storage.NewQueuedMessage("second prompt", []string{"abc123"}),
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.QueuedMessages) != 2 || loaded.QueuedMessages[0].Text != "one prompt" || loaded.QueuedMessages[1].Text != "second prompt" {
		t.Fatalf("queued messages = %#v", loaded.QueuedMessages)
	}
	if got := loaded.QueuedMessages[1].AttachmentIDs; len(got) != 1 || got[0] != "abc123" {
		t.Fatalf("queued attachment ids = %#v", got)
	}
}

func TestAddUsageStoresCachedTokens(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "usage"})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.AddUsage(session.ID, storage.Usage{
		InputTokens:           100,
		CachedInputTokens:     64,
		CachedWriteTokens:     6,
		OutputTokens:          25,
		ReasoningOutputTokens: 7,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.AddUsage(session.ID, storage.Usage{
		InputTokens:       10,
		CachedInputTokens: 8,
		OutputTokens:      5,
		TotalTokens:       20,
	}); err != nil {
		t.Fatal(err)
	}

	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	// Missing totals derive from input + output; cache fields are detail.
	if loaded.Usage.InputTokens != 110 || loaded.Usage.CachedInputTokens != 72 || loaded.Usage.CachedWriteTokens != 6 ||
		loaded.Usage.OutputTokens != 30 || loaded.Usage.ReasoningOutputTokens != 7 || loaded.Usage.TotalTokens != 145 {
		t.Fatalf("usage = %#v", loaded.Usage)
	}
	// Context reflects only the latest turn's input + output, never accumulates.
	if loaded.Usage.ContextTokens != 15 {
		t.Fatalf("context tokens = %d, want 15", loaded.Usage.ContextTokens)
	}
	daily, err := usagecore.NewService(store).Daily(usagecore.DailyQuery{Days: 1, Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	wantDaily := usagecore.UsageTotals{
		InputTokens:           loaded.Usage.InputTokens,
		CachedInputTokens:     loaded.Usage.CachedInputTokens,
		CachedWriteTokens:     loaded.Usage.CachedWriteTokens,
		OutputTokens:          loaded.Usage.OutputTokens,
		ReasoningOutputTokens: loaded.Usage.ReasoningOutputTokens,
	}
	if len(daily) != 1 || daily[0].SessionCount != 1 || daily[0].Usage != wantDaily {
		t.Fatalf("daily usage = %#v, want one bucket with %#v", daily, wantDaily)
	}
}

func TestDailyIgnoresImportedSessionUsageFallback(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "imported-usage"})
	if err != nil {
		t.Fatal(err)
	}
	session.Usage = storage.Usage{
		InputTokens:       1000,
		CachedInputTokens: 2000,
		OutputTokens:      3000,
		TotalTokens:       6000,
	}
	if err := store.SaveSession(session); err != nil {
		t.Fatal(err)
	}

	daily, err := usagecore.NewService(store).Daily(usagecore.DailyQuery{Days: 1, Location: time.UTC})
	if err != nil {
		t.Fatal(err)
	}
	if len(daily) != 1 || daily[0].SessionCount != 0 || daily[0].Usage != (usagecore.UsageTotals{}) {
		t.Fatalf("daily usage from imported session fallback = %#v", daily)
	}
}

func TestSaveACPStateUpdatesSessionStatus(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "acp", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}

	if err := store.SaveACPState(session.ID, storage.ACPState{State: "running"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusRunning {
		t.Fatalf("status = %q, want %q", loaded.Status, storage.StatusRunning)
	}

	if err := store.SaveACPState(session.ID, storage.ACPState{State: "failed", Error: "codex failed"}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusError {
		t.Fatalf("status = %q, want %q", loaded.Status, storage.StatusError)
	}
	if loaded.Error != "codex failed" {
		t.Fatalf("error = %q, want %q", loaded.Error, "codex failed")
	}

	if err := store.SaveACPState(session.ID, storage.ACPState{State: "idle"}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != storage.StatusIdle {
		t.Fatalf("status = %q, want %q", loaded.Status, storage.StatusIdle)
	}
	if loaded.Error != "" {
		t.Fatalf("error = %q, want empty", loaded.Error)
	}
}

func TestSaveACPStateDoesNotDuplicateTranscript(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "acp-state", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	state := storage.ACPState{
		State:       "running",
		Assistant:   "live answer",
		Thought:     "live thought",
		Plan:        []sessionevents.PlanEntry{{Content: "live plan"}},
		ToolCalls:   []sessionevents.ACPToolCall{{ID: "tool-1", RawOutput: []byte("large output")}},
		Permissions: []sessionevents.ACPPermission{{ID: "permission-1"}},
	}
	if err := store.SaveACPState(session.ID, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadACPState(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Assistant != "" || loaded.Thought != "" || loaded.Plan != nil || loaded.ToolCalls != nil || loaded.Permissions != nil {
		t.Fatalf("stored state retained transcript copies: %#v", loaded)
	}
	state.State = "idle"
	if err := store.SaveACPState(session.ID, state); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadACPState(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Assistant != "" || loaded.Thought != "" || loaded.Plan != nil || loaded.ToolCalls != nil || loaded.Permissions != nil {
		t.Fatalf("inactive state retained transcript copies: %#v", loaded)
	}
}

func TestCompactACPStatesRewritesLegacyTranscriptCopies(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "legacy-acp-state", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	legacy := storage.ACPState{
		ID:    session.ID,
		State: "idle",
		ToolCalls: []sessionevents.ACPToolCall{{
			ID:        "tool-1",
			RawOutput: stdjson.RawMessage(`"` + strings.Repeat("x", largeACPStateBytes) + `"`),
		}},
	}
	raw, err := stdjson.MarshalIndent(legacy, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.sessionDir(session.ID), "acp_state.json")
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	files, removed, err := store.CompactACPStates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if files != 1 || removed <= 0 {
		t.Fatalf("compaction = %d files, %d bytes", files, removed)
	}
	state, err := store.LoadACPState(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.ToolCalls != nil || state.ID != session.ID || state.State != "idle" {
		t.Fatalf("state = %#v", state)
	}
}

func TestCompactACPStatesContinuesPastCorruptFile(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	corrupt, err := store.CreateSession(storage.CreateSession{Slug: "a-corrupt-state", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := store.CreateSession(storage.CreateSession{Slug: "z-healthy-state", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	large := strings.Repeat("x", largeACPStateBytes)
	if err := os.WriteFile(filepath.Join(store.sessionDir(corrupt.ID), "acp_state.json"), []byte("{"+large), 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := storage.ACPState{ID: healthy.ID, State: "idle", Assistant: large}
	raw, err := stdjson.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.sessionDir(healthy.ID), "acp_state.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	files, removed, err := store.CompactACPStates(t.Context())
	if err == nil || files != 1 || removed <= 0 {
		t.Fatalf("compaction = %d files, %d bytes, %v", files, removed, err)
	}
	state, err := store.LoadACPState(healthy.ID)
	if err != nil {
		t.Fatal(err)
	}
	if state.Assistant != "" || state.ID != healthy.ID {
		t.Fatalf("healthy state = %#v", state)
	}
}

func TestMessagesUseJSONL(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	session, err := store.CreateSession(storage.CreateSession{Slug: "jsonl"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMessages(session.ID, []provider.Message{
		provider.UserMessage("hello"),
		provider.AssistantMessage("done", nil),
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(store.sessionDir(session.ID), "messages.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store.sessionDir(session.ID), "messages.json")); !os.IsNotExist(err) {
		t.Fatalf("messages.json exists or stat failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("jsonl lines = %d, want 2", len(lines))
	}
	for _, line := range lines {
		var msg provider.Message
		if err := stdjson.Unmarshal([]byte(line), &msg); err != nil {
			t.Fatalf("invalid jsonl line %q: %v", line, err)
		}
	}
	loaded, err := store.LoadMessages(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || provider.MessageContent(loaded[1]) != "done" {
		t.Fatalf("loaded messages = %#v", loaded)
	}

	if err := store.AppendMessages(session.ID, provider.UserMessage("again")); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(store.sessionDir(session.ID), "messages.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	lines = strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("jsonl lines after append = %d, want 3", len(lines))
	}
}

func TestSessionEventsUseJSONL(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
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
	if len(loaded) != 3 {
		t.Fatalf("events = %d, want 3", len(loaded))
	}
	if loaded[0].Seq != 1 || loaded[0].SessionID != session.ID || loaded[0].Content != "working" {
		t.Fatalf("first event = %#v", loaded[0])
	}
	if loaded[1].Seq != 2 || loaded[1].ACP == nil || loaded[1].ACP.ToolCalls[0].Title != "Read file" {
		t.Fatalf("second event = %#v", loaded[1])
	}
	if loaded[2].Seq != 3 || loaded[2].Plan == nil || loaded[2].Plan.Plan[0].Content != "Run tests" {
		t.Fatalf("third event = %#v", loaded[2])
	}
	if loaded[0].At.IsZero() || loaded[1].At.IsZero() || loaded[2].At.IsZero() {
		t.Fatalf("events should be timestamped: %#v", loaded)
	}

	loaded, err = store.LoadSessionEventsAfter(session.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[0].Seq != 2 || loaded[1].Seq != 3 {
		t.Fatalf("events after seq 1 = %#v", loaded)
	}
}

func TestAssignedSessionEventAppendDoesNotReadHistory(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "assigned-events"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.sessionDir(session.ID), "events.jsonl")
	if err := os.WriteFile(path, []byte("not-json\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Seq: 9, Type: "acp_message", Content: "new"}); err != nil {
		t.Fatalf("append assigned event: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"seq":9`) {
		t.Fatalf("assigned event was not appended: %s", data)
	}
}

func TestUnassignedSessionEventFollowsHighestSequence(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	session, err := store.CreateSession(storage.CreateSession{Slug: "event-sequence"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Seq: 9, Type: "acp_message", Content: "assigned"}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{Type: "acp_message", Content: "next"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 2 || loaded[1].Seq != 10 {
		t.Fatalf("events = %#v, want second sequence 10", loaded)
	}
}
