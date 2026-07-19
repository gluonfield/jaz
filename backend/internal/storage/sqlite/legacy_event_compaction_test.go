package sqlite

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestLegacySessionEventCompactionRunsOnce(t *testing.T) {
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
		session.ID, `{"id":"agent-1","state":"running","title":"latest"}`,
	); err != nil {
		t.Fatal(err)
	}
	result, err := store.CompactNextLegacySessionEvents(t.Context())
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
	if len(events) != 3 || events[0].Content != "Hel" || events[1].Content != "lo" || events[2].Seq != 4 || events[2].ACP == nil || events[2].ACP.Title != "latest" {
		t.Fatalf("compacted events = %#v", events)
	}
	if next, err := store.CompactNextLegacySessionEvents(t.Context()); err != nil || next.ThreadID != "" {
		t.Fatalf("second compaction = %#v, %v", next, err)
	}
}

func TestLegacySessionEventCompactionPrioritizesLargestThread(t *testing.T) {
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
	result, err := store.CompactNextLegacySessionEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if result.ThreadID != large.ID || !result.Complete {
		t.Fatalf("first compaction = %#v, want largest %q", result, large.ID)
	}
}

func TestLegacySessionEventCompactionPreservesConcurrentAppend(t *testing.T) {
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
	plan, err := store.loadLegacyEventPlan(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, sessionevents.Event{
		Type: "acp_message", Content: "!", ACP: &sessionevents.ACPEvent{ID: "agent-1", State: "idle"},
	}); err != nil {
		t.Fatal(err)
	}
	if result, current, err := store.applyLegacyEventPlan(t.Context(), plan); err != nil || current || result.Removed != 0 {
		t.Fatalf("stale compaction = %#v, current %t, err %v", result, current, err)
	}
	result, err := store.CompactNextLegacySessionEvents(t.Context())
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

func TestLegacySessionEventCompactionSkipsCorruptThread(t *testing.T) {
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
	result, err := store.CompactNextLegacySessionEvents(t.Context())
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
	result, err = store.CompactNextLegacySessionEvents(t.Context())
	if err != nil || result.ThreadID != good.ID || !result.Complete {
		t.Fatalf("good compaction = %#v, %v", result, err)
	}
}

func TestLegacySessionEventPreparationDoesNotTakeWriterMutex(t *testing.T) {
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
		_, err := store.loadLegacyEventPlan(context.Background())
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
		t.Fatal("legacy event preparation waited for the writer mutex")
	}
}

func TestLegacySessionEventCompactionUsesBoundedWriteBatches(t *testing.T) {
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
		result, err := store.CompactNextLegacySessionEvents(t.Context())
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
		if steps > 4 {
			t.Fatalf("compaction did not converge after %d steps", steps)
		}
	}
	if steps != 3 || totalRemoved != 129 {
		t.Fatalf("steps = %d, removed = %d; want 3 and 129", steps, totalRemoved)
	}
	events, err := store.LoadSessionEvents(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Seq != 130 {
		t.Fatalf("compacted events = %#v", events)
	}
}
