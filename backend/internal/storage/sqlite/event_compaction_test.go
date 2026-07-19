package sqlite

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestSessionEventCompactionRunsOnce(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "legacy-events"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE threads SET event_compaction_version = 0 WHERE id = ?`, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO session_events (thread_id, seq, type, content, acp, created_at_ms, coalesce_key)
		VALUES (?, 1, 'acp_message', 'Hel', ?, 1, ''),
		       (?, 2, 'acp_message', 'lo', ?, 2, ''),
		       (?, 3, 'acp', '', ?, 3, ''),
		       (?, 4, 'acp', '', ?, 4, '')`,
		session.ID, `{"id":"agent-1","state":"running"}`,
		session.ID, `{"id":"agent-1","state":"running"}`,
		session.ID, `{"id":"agent-1","state":"running","title":"first"}`,
		session.ID, `{"id":"agent-1","state":"running","title":"latest","assistant":"duplicate","thought":"duplicate","tool_calls":[{"id":"tool-1"}]}`,
	); err != nil {
		t.Fatal(err)
	}
	result, err := store.CompactNextSessionEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID != session.ID || result.Removed != 1 || !result.Complete {
		t.Fatalf("compaction = %#v", result)
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Content != "Hel" || events[1].Content != "lo" || events[2].Seq != 4 || events[2].ACP == nil ||
		events[2].ACP.Title != "" || events[2].ACP.Assistant != "" || events[2].ACP.Thought != "" || len(events[2].ACP.ToolCalls) != 0 {
		t.Fatalf("compacted events = %#v", events)
	}
	if next, err := store.CompactNextSessionEvents(t.Context()); err != nil || next.ThreadID != "" {
		t.Fatalf("second compaction = %#v, %v", next, err)
	}
}

func TestCompletedTextRunIsCompactedOffTheWritePath(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "text-run"})
	if err != nil {
		t.Fatal(err)
	}
	text := func(content string) sessionevents.Event {
		return sessionevents.Event{
			Type: sessionevents.TypeACPMessage, Content: content,
			ACP: &sessionevents.ACPEvent{ID: session.ID, State: "running", TextRunID: "message:one"},
		}
	}
	if err := store.AppendSessionEvents(session.ID, text("Hel"), text("lo")); err != nil {
		t.Fatal(err)
	}
	var version int
	if err := store.db.QueryRow(`SELECT event_compaction_version FROM threads WHERE id = ?`, session.ID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 0 {
		t.Fatalf("text append left compaction version %d, want pending", version)
	}
	select {
	case <-store.SessionEventCompactionWake():
	default:
		t.Fatal("text append did not wake background compaction")
	}

	result, err := store.CompactNextSessionEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID != session.ID || result.Removed != 1 || !result.Complete {
		t.Fatalf("compaction = %#v", result)
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Seq != 2 || events[0].Content != "Hello" || events[0].ProjectionKey == "" {
		t.Fatalf("compacted text = %#v", events)
	}
}

func TestTerminalSessionTransitionsWakeDeferredCompaction(t *testing.T) {
	transitions := map[string]func(*Store, storage.Session) error{
		"save": func(store *Store, session storage.Session) error {
			session.Status = storage.StatusIdle
			return store.SaveSession(session)
		},
		"status": func(store *Store, session storage.Session) error {
			return store.UpdateSessionStatus(session.ID, storage.StatusError, "failed", time.Now().UTC())
		},
		"complete": func(store *Store, session storage.Session) error {
			return store.CompleteSession(session.ID, time.Now().UTC())
		},
	}
	for name, transition := range transitions {
		t.Run(name, func(t *testing.T) {
			store, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			session, err := store.CreateSession(storage.CreateSession{Slug: "deferred-" + name})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.UpdateSessionStatus(session.ID, storage.StatusRunning, "", time.Time{}); err != nil {
				t.Fatal(err)
			}
			if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
				Type: sessionevents.TypeACPMessage, Content: "pending",
				ACP: &sessionevents.ACPEvent{ID: session.ID, State: "running", TextRunID: "message:one"},
			}); err != nil {
				t.Fatal(err)
			}
			select {
			case <-store.SessionEventCompactionWake():
			default:
				t.Fatal("projectable append did not wake compaction")
			}
			if result, err := store.CompactNextSessionEvents(t.Context()); err != nil || result.ThreadID != "" {
				t.Fatalf("running compaction = %#v, %v", result, err)
			}
			if err := transition(store, session); err != nil {
				t.Fatal(err)
			}
			select {
			case <-store.SessionEventCompactionWake():
			default:
				t.Fatal("terminal transition did not wake deferred compaction")
			}
			result, err := store.CompactNextSessionEvents(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if result.ThreadID != session.ID || !result.Complete {
				t.Fatalf("terminal compaction = %#v", result)
			}
		})
	}
}

func TestSessionEventCompactionPrioritizesLargestThread(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	small, err := store.CreateSession(storage.CreateSession{Slug: "legacy-small"})
	if err != nil {
		t.Fatal(err)
	}
	large, err := store.CreateSession(storage.CreateSession{Slug: "legacy-large"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO session_events (thread_id, seq, type, content, created_at_ms, coalesce_key)
		VALUES (?, 1, 'acp_message', 'small', 1, ''),
		       (?, 1, 'acp_message', 'one', 1, ''),
		       (?, 2, 'acp_message', 'two', 2, ''),
		       (?, 3, 'acp_message', 'three', 3, '');
		UPDATE threads
		SET event_compaction_version = 0,
		    event_revision = (SELECT COUNT(*) FROM session_events WHERE thread_id = threads.id)
		WHERE id IN (?, ?)`, small.ID, large.ID, large.ID, large.ID, small.ID, large.ID); err != nil {
		t.Fatal(err)
	}
	result, err := store.CompactNextSessionEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID != large.ID || !result.Complete {
		t.Fatalf("first compaction = %#v, want largest %q", result, large.ID)
	}
}

func TestSessionEventCompactionPreservesConcurrentAppend(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "legacy-concurrent"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE threads SET event_compaction_version = 0 WHERE id = ?`, session.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO session_events (thread_id, seq, type, content, acp, created_at_ms, coalesce_key)
		VALUES (?, 1, 'acp_message', 'Hel', ?, 1, ''),
		       (?, 2, 'acp_message', 'lo', ?, 2, '')`,
		session.ID, `{"id":"agent-1","state":"running"}`,
		session.ID, `{"id":"agent-1","state":"running"}`,
	); err != nil {
		t.Fatal(err)
	}
	plan, err := store.loadSessionEventCompactionPlan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: "acp_message", Content: "!", ACP: &sessionevents.ACPEvent{ID: "agent-1", State: "idle"},
	}); err != nil {
		t.Fatal(err)
	}
	if result, current, err := store.applySessionEventCompactionPlan(t.Context(), plan); err != nil || current || result.Removed != 0 {
		t.Fatalf("stale compaction = %#v, current %t, err %v", result, current, err)
	}
	result, err := store.CompactNextSessionEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID != session.ID || result.Removed != 0 || !result.Complete {
		t.Fatalf("compaction = %#v", result)
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Content != "Hel" || events[1].Content != "lo" || events[2].Content != "!" || events[2].ACP.State != "idle" {
		t.Fatalf("compacted events = %#v", events)
	}
}

func TestSessionEventCompactionSkipsCorruptThread(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bad, err := store.CreateSession(storage.CreateSession{Slug: "legacy-bad"})
	if err != nil {
		t.Fatal(err)
	}
	good, err := store.CreateSession(storage.CreateSession{Slug: "legacy-good"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		UPDATE threads
		SET event_compaction_version = 0,
		    updated_at_ms = CASE id WHEN ? THEN 1 ELSE 2 END
		WHERE id IN (?, ?)`, bad.ID, bad.ID, good.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO session_events (thread_id, seq, type, content, acp, created_at_ms, coalesce_key)
		VALUES (?, 1, 'acp_message', 'bad', '{', 1, ''),
		       (?, 1, 'acp_message', 'good', ?, 1, '')`,
		bad.ID, good.ID, `{"id":"agent-2","state":"idle"}`,
	); err != nil {
		t.Fatal(err)
	}
	result, err := store.CompactNextSessionEvents(t.Context())
	if result.ThreadID != bad.ID || err == nil {
		t.Fatalf("bad compaction = %#v, %v", result, err)
	}
	var version int
	if err := store.db.QueryRow(`SELECT event_compaction_version FROM threads WHERE id = ?`, bad.ID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("bad compaction version = %d, want skipped", version)
	}
	result, err = store.CompactNextSessionEvents(t.Context())
	if err != nil || result.ThreadID != good.ID || !result.Complete {
		t.Fatalf("good compaction = %#v, %v", result, err)
	}
}

func TestSessionEventCompactionPreparationDoesNotTakeWriterMutex(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "legacy-read-outside-writer-lock"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE threads SET event_compaction_version = 0 WHERE id = ?`, session.ID); err != nil {
		t.Fatal(err)
	}
	store.writeMu.Lock()
	done := make(chan error, 1)
	go func() {
		_, err := store.loadSessionEventCompactionPlan(context.Background())
		done <- err
	}()
	select {
	case err := <-done:
		store.writeMu.Unlock()
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		store.writeMu.Unlock()
		t.Fatal("event compaction preparation waited for the writer mutex")
	}
}

func TestSessionEventCompactionYieldsToLiveWriter(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "live-writer-priority"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID,
		sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "a", ACP: &sessionevents.ACPEvent{ID: session.ID, TextRunID: "message:one"}},
		sessionevents.Event{Type: sessionevents.TypeACPMessage, Content: "b", ACP: &sessionevents.ACPEvent{ID: session.ID, TextRunID: "message:one"}},
	); err != nil {
		t.Fatal(err)
	}

	store.writeMu.Lock()
	done := make(chan SessionEventCompaction, 1)
	go func() {
		result, _ := store.CompactNextSessionEvents(t.Context())
		done <- result
	}()
	select {
	case result := <-done:
		store.writeMu.Unlock()
		if result.ThreadID != session.ID || result.Removed != 0 || result.Complete {
			t.Fatalf("yield result = %#v", result)
		}
	case <-time.After(time.Second):
		store.writeMu.Unlock()
		t.Fatal("maintenance waited behind a live writer")
	}
	result, err := store.CompactNextSessionEvents(t.Context())
	if err != nil || !result.Complete {
		t.Fatalf("resumed compaction = %#v, %v", result, err)
	}
}

func TestSessionEventCompactionUsesBoundedWriteBatches(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "legacy-batches"})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for seq := 1; seq <= 130; seq++ {
		acp := fmt.Sprintf(`{"id":"agent-1","state":"running","title":"update-%d"}`, seq)
		if _, err := tx.Exec(`
			INSERT INTO session_events (thread_id, seq, type, content, acp, created_at_ms, coalesce_key)
			VALUES (?, ?, 'acp', '', ?, ?, '')`, session.ID, seq, acp, seq); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(`
		UPDATE threads
		SET event_compaction_version = 0, event_revision = 130
		WHERE id = ?`, session.ID); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	totalRemoved := 0
	steps := 0
	for {
		result, err := store.CompactNextSessionEvents(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		steps++
		totalRemoved += result.Removed
		if steps == 1 && (result.Complete || result.Removed >= 130) {
			t.Fatalf("first batch was not bounded: %#v", result)
		}
		if result.Complete {
			break
		}
		if steps > 1+(130+eventCompactionBatchOperations-1)/eventCompactionBatchOperations {
			t.Fatalf("compaction did not converge after %d steps", steps)
		}
	}
	if steps < 2 || totalRemoved != 129 {
		t.Fatalf("steps = %d, removed = %d; want multiple bounded steps and 129 removals", steps, totalRemoved)
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Seq != 130 {
		t.Fatalf("compacted events = %#v", events)
	}
}

func TestSessionEventCompactionSkipsLegacyRowsAboveMaintenanceBudget(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "projection-byte-batches"})
	if err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("x", 11<<20)
	tx, err := store.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for seq := 1; seq <= 3; seq++ {
		acp := fmt.Sprintf(`{"id":"agent","state":"running","text_run_id":"message:%d"}`, seq)
		if _, err := tx.Exec(`
			INSERT INTO session_events (thread_id, seq, type, content, acp, created_at_ms, coalesce_key)
			VALUES (?, ?, 'acp_message', ?, ?, ?, '')`, session.ID, seq, content, acp, seq); err != nil {
			_ = tx.Rollback()
			t.Fatal(err)
		}
	}
	if _, err := tx.Exec(`UPDATE threads SET event_compaction_version = 0, event_revision = 3 WHERE id = ?`, session.ID); err != nil {
		_ = tx.Rollback()
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	result, err := store.CompactNextSessionEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete {
		t.Fatalf("oversized legacy compaction did not stop: %#v", result)
	}
	var projected int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM session_events WHERE thread_id = ? AND projection_key <> ''`, session.ID).Scan(&projected); err != nil {
		t.Fatal(err)
	}
	if projected != 0 {
		t.Fatalf("oversized legacy compaction rewrote %d rows", projected)
	}
	var version int
	if err := store.db.QueryRow(`SELECT event_compaction_version FROM threads WHERE id = ?`, session.ID).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 2 {
		t.Fatalf("compaction version = %d, want skipped version 2", version)
	}
}

func TestTextCompactionResumesWithoutDuplicatingSnapshot(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "text-restart"})
	if err != nil {
		t.Fatal(err)
	}
	events := make([]sessionevents.Event, 130)
	for i := range events {
		events[i] = sessionevents.Event{
			Type: sessionevents.TypeACPMessage, Content: "x",
			ACP: &sessionevents.ACPEvent{ID: session.ID, State: "running", TextRunID: "message:one"},
		}
	}
	if err := store.AppendSessionEvents(session.ID, events...); err != nil {
		t.Fatal(err)
	}
	first, err := store.CompactNextSessionEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if first.Complete || first.Removed == 0 {
		t.Fatalf("first compaction was not interrupted: %#v", first)
	}
	store.eventPlanner.mu.Lock()
	store.eventPlanner.plan = nil
	store.eventPlanner.mu.Unlock()

	for steps := 0; ; steps++ {
		result, err := store.CompactNextSessionEvents(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if result.Complete {
			break
		}
		if steps > (len(events)+eventCompactionBatchOperations-1)/eventCompactionBatchOperations+4 {
			t.Fatal("resumed compaction did not converge")
		}
	}
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	projected := sessionevents.CompactTranscript(stored)
	if len(projected) != 1 || projected[0].Content != strings.Repeat("x", len(events)) {
		t.Fatalf("resumed text snapshot = %#v", stored)
	}
}

func TestSessionEventCompactionBoundsTextSnapshots(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Slug: "bounded-text"})
	if err != nil {
		t.Fatal(err)
	}
	const chunks = 100
	chunk := strings.Repeat("x", 8<<10)
	events := make([]sessionevents.Event, chunks)
	for i := range events {
		events[i] = sessionevents.Event{
			Type: sessionevents.TypeACPMessage, Content: chunk,
			ACP: &sessionevents.ACPEvent{ID: session.ID, State: "running", TextRunID: "message:bounded"},
		}
	}
	if err := store.AppendSessionEvents(session.ID, events...); err != nil {
		t.Fatal(err)
	}
	for steps := 0; ; steps++ {
		result, err := store.CompactNextSessionEvents(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if result.Complete {
			break
		}
		if steps > (len(events)+eventCompactionBatchOperations-1)/eventCompactionBatchOperations+4 {
			t.Fatal("bounded text compaction did not converge")
		}
	}
	stored, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) < 2 {
		t.Fatalf("text run was stored as one unbounded snapshot: %d rows", len(stored))
	}
	for _, event := range stored {
		if len(event.Content) > eventCompactionTextBytes {
			t.Fatalf("snapshot has %d bytes, limit %d", len(event.Content), eventCompactionTextBytes)
		}
	}
	projected := sessionevents.CompactTranscript(stored)
	if len(projected) != 1 {
		t.Fatalf("projected text = %d events", len(projected))
	}
	if len(projected[0].Content) != chunks*len(chunk) {
		t.Fatalf("projected text = %d bytes", len(projected[0].Content))
	}
}
