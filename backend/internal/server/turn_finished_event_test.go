package server

import (
	"context"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

type heldTurnFinishStore struct {
	*sqlitestore.Store
	entered chan struct{}
	release chan struct{}
}

func (s *heldTurnFinishStore) FinishSessionTurn(id, status, message string, at time.Time, deliveries []storage.TurnDelivery) error {
	close(s.entered)
	<-s.release
	return s.Store.FinishSessionTurn(id, status, message, at, deliveries)
}

func TestTurnFinishedIsPublishedAfterTheResultIsSaved(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	session, err := store.CreateSession(storage.CreateSession{Title: "Research", Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.ClaimTurn(session.ID, storage.Turn{StartedAt: time.Now()})
	if err != nil || !claimed {
		t.Fatalf("claim turn: %v %v", claimed, err)
	}
	held := &heldTurnFinishStore{Store: store, entered: make(chan struct{}), release: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := sessionevents.New()
	events := bus.Subscribe(ctx, session.ID, sessionevents.TypeTurnFinished)
	server := &Server{Store: held, Events: bus, Locks: sessionlock.New()}
	done := make(chan struct{})
	go func() {
		defer close(done)
		server.HandleACPTurnFinished(ctx, acp.Job{ID: session.ID, State: acp.StateIdle, StopReason: acp.StopReasonEndTurn})
	}()
	<-held.entered
	select {
	case event := <-events:
		close(held.release)
		<-done
		t.Fatalf("published completion before saving: %#v", event)
	default:
	}
	close(held.release)
	<-done
	select {
	case event := <-events:
		saved, err := store.LoadSession(event.SessionID)
		if err != nil || saved.Turn != nil || saved.Status != storage.StatusIdle || saved.CompletedAt.IsZero() {
			t.Fatalf("completion has no saved result: %#v %v", saved, err)
		}
	default:
		t.Fatal("saved completion was not published")
	}
}
