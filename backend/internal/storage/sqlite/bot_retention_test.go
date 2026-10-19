package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestFinishedBotWorkArchivesOnlyAfterClosureAndInactivity(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bot, err := store.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	worktreeRoot := filepath.Join(t.TempDir(), ".worktrees")
	now := time.Now().UTC()
	before := now.Add(-7 * 24 * time.Hour)
	old := before.Add(-time.Hour)
	for _, tc := range []struct {
		name        string
		mutate      func(*storage.Session)
		closed      bool
		recentClose bool
		want        bool
	}{
		{name: "finished", closed: true, want: true},
		{name: "open"},
		{name: "recent closure", closed: true, recentClose: true},
		{name: "pinned", closed: true, mutate: func(s *storage.Session) {
			s.Pinned = true
		}},
		{name: "new prompt", closed: true, mutate: func(s *storage.Session) {
			s.LastAttentionAt = now
		}},
		{name: "new completion", closed: true, mutate: func(s *storage.Session) {
			s.CompletedAt = now
		}},
		{name: "recent edit", closed: true, mutate: func(s *storage.Session) {
			s.UpdatedAt = now
		}},
		{name: "running", closed: true, mutate: func(s *storage.Session) {
			s.Status = storage.StatusRunning
		}},
		{name: "interrupted", closed: true, mutate: func(s *storage.Session) {
			s.Status = storage.StatusInterrupted
		}},
		{name: "queued", closed: true, mutate: func(s *storage.Session) {
			s.QueuedMessages = []storage.QueuedMessage{storage.NewInternalQueuedMessage("next")}
		}},
		{name: "outside managed root", closed: true, mutate: func(s *storage.Session) {
			s.RuntimeRef.Cwd = worktreeRoot + "-other/work"
		}},
		{name: "uncompleted", closed: true, mutate: func(s *storage.Session) {
			s.CompletedAt = time.Time{}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			commitment, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: tc.name})
			if err != nil {
				t.Fatal(err)
			}
			worker, err := store.CreateSession(storage.CreateSession{Slug: tc.name, ParentID: bot.ID, SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: commitment.ID, RuntimeRef: &storage.RuntimeRef{Cwd: filepath.Join(worktreeRoot, tc.name)}})
			if err != nil {
				t.Fatal(err)
			}
			if tc.closed {
				closedAt := old
				if tc.recentClose {
					closedAt = now
				}
				if err := store.DropCommitment(commitment.ID, "finished", closedAt); err != nil {
					t.Fatal(err)
				}
			}
			worker.UpdatedAt = old
			worker.LastAttentionAt = old
			worker.CompletedAt = old
			if tc.mutate != nil {
				tc.mutate(&worker)
			}
			if err := store.CompleteSession(worker.ID, worker.CompletedAt); err != nil {
				t.Fatal(err)
			}
			if err := store.saveSessionLocked(worker, false); err != nil {
				t.Fatal(err)
			}
			child, err := store.CreateSession(storage.CreateSession{Slug: tc.name + "-review", ParentID: worker.ID, SourceType: storage.SourceBotWorker, SourceID: bot.ID, RuntimeRef: &storage.RuntimeRef{Cwd: filepath.Join(worktreeRoot, tc.name+"-review")}})
			if err != nil {
				t.Fatal(err)
			}
			child.UpdatedAt = old
			child.LastAttentionAt = old
			child.CompletedAt = old
			if err := store.CompleteSession(child.ID, old); err != nil {
				t.Fatal(err)
			}
			if err := store.saveSessionLocked(child, false); err != nil {
				t.Fatal(err)
			}
			if err := store.ArchiveFinishedBotWork(t.Context(), before, worktreeRoot); err != nil {
				t.Fatal(err)
			}
			got, err := store.LoadSession(worker.ID)
			if err != nil || got.Archived != tc.want {
				t.Fatalf("archived = %v, want %v: %v", got.Archived, tc.want, err)
			}
			got, err = store.LoadSession(child.ID)
			if want := tc.closed && !tc.recentClose; err != nil || got.Archived != want {
				t.Fatalf("descendant archived = %v, want %v: %v", got.Archived, want, err)
			}
		})
	}
}
