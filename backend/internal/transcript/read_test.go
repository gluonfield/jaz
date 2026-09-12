package transcript

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

type readStore struct {
	session  storage.Session
	page     storage.TranscriptPage
	related  storage.TranscriptSessions
	parentID string
	ids      []string
}

type emptyLiveReader struct{}

func (emptyLiveReader) HydrationJobs([]string) map[string]acp.HydrationView { return nil }

type liveReader map[string]acp.HydrationView

func (r liveReader) HydrationJobs([]string) map[string]acp.HydrationView { return r }

func TestLoadPreservesClaimedTurnBeforeRuntimeStarts(t *testing.T) {
	for name, live := range map[string]liveReader{
		"first turn": nil,
		"next turn":  {"thread": {ID: "thread", State: acp.StateIdle, StopReason: "end_turn"}},
	} {
		t.Run(name, func(t *testing.T) {
			store := &readStore{session: storage.Session{ID: "thread", Runtime: storage.RuntimeACP, Status: storage.StatusRunning}}
			view, err := NewService(store, live).Load(t.Context(), "thread", storage.TranscriptPageRequest{})
			if err != nil {
				t.Fatal(err)
			}
			if view.Session.Status != storage.StatusRunning {
				t.Fatalf("claimed turn reported %q before its prompt or runtime job exists", view.Session.Status)
			}
		})
	}
}

func (s *readStore) LoadTranscriptSession(context.Context, string) (storage.Session, error) {
	return s.session, nil
}

func (s *readStore) LoadTranscriptPage(context.Context, string, storage.TranscriptPageRequest) (storage.TranscriptPage, error) {
	return s.page, nil
}

func (s *readStore) LoadTranscriptSessions(_ context.Context, parentID string, ids []string) (storage.TranscriptSessions, error) {
	s.parentID = parentID
	s.ids = append([]string(nil), ids...)
	return s.related, nil
}

func TestLoadBuildsOneBoundedReadModel(t *testing.T) {
	now := time.Now().UTC()
	session := storage.Session{
		ID: "parent", LastAttentionAt: now,
		PendingSteer: &storage.QueuedMessage{Text: "latest"},
	}
	store := &readStore{
		session: session,
		page: storage.TranscriptPage{
			Messages: []storage.Message{{Role: "user", Content: "latest", CreatedAt: now}},
			Events: []sessionevents.Event{
				{ACP: &sessionevents.ACPEvent{ID: "child"}},
				{ACP: &sessionevents.ACPEvent{ID: "reference"}},
				{ACP: &sessionevents.ACPEvent{ID: "reference"}},
			},
		},
		related: storage.TranscriptSessions{
			Children:   []storage.TranscriptSession{{ID: "child"}},
			References: []storage.TranscriptSession{{ID: "reference", Title: "Reference"}},
		},
	}
	view, err := NewService(store, emptyLiveReader{}).Load(t.Context(), session.ID, storage.TranscriptPageRequest{Turns: 14})
	if err != nil {
		t.Fatal(err)
	}
	if store.parentID != session.ID || !slices.Equal(store.ids, []string{"child", "reference"}) {
		t.Fatalf("related read = parent %q, ids %#v", store.parentID, store.ids)
	}
	if view.Session.PendingSteer != nil || len(view.Children) != 1 || view.Meta["reference"].Title != "Reference" {
		t.Fatalf("view = %#v", view)
	}
}
