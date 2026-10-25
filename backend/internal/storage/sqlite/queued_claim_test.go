package sqlite

import (
	"errors"
	"slices"
	"testing"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestQueuedClaimRejectsSnapshotRetiredByStopAndResume(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bot, err := s.CreateSession(storage.CreateSession{SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Find a usable route"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := s.CreateSession(storage.CreateSession{SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	old := storage.QueuedMessage{Text: "Review this input", From: bot.ID}
	for _, message := range []storage.QueuedMessage{old, storage.NewQueuedMessage("Unrelated queued work", nil)} {
		if err := s.QueueTurn(storage.TurnDelivery{SessionID: worker.ID, Message: message}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot, err := s.LoadSession(worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	selected := snapshot.QueuedMessages[0]
	snapshot.QueuedMessages = snapshot.QueuedMessages[1:]
	for _, change := range []struct {
		status string
		reason string
	}{
		{storage.CommitmentStopped, "Both supported routes reject the input; no useful retry condition is known."},
		{storage.CommitmentPending, "The user supplied a supported input; review it with fresh instructions."},
	} {
		if _, err := s.TransitionCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, Status: change.status}, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: change.reason}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.QueueTurn(storage.TurnDelivery{SessionID: worker.ID, Message: old}); err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimQueuedTurn(snapshot, selected); !errors.Is(err, storage.ErrQueueChanged) {
		t.Fatalf("retired selection admitted after resume: %v", err)
	}
	current, err := s.LoadSession(worker.ID)
	if err != nil || current.Status != storage.StatusIdle || current.Turn != nil || len(current.QueuedMessages) != 2 || current.QueuedMessages[0].Text != "Unrelated queued work" || current.QueuedMessages[1].ID == selected.ID {
		t.Fatalf("stale claim restored retired intent or lost fresh work: %+v, %v", current, err)
	}
	user := current.QueuedMessages[0]
	if err := s.ClaimQueuedTurn(storage.Session{ID: worker.ID}, user); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.LoadSession(worker.ID)
	if err != nil || claimed.Turn == nil || claimed.Turn.PendingMessage.ID != user.ID || len(claimed.QueuedMessages) != 1 || claimed.QueuedMessages[0].ID != current.QueuedMessages[1].ID {
		t.Fatalf("claim did not derive the saved remainder: %+v, %v", claimed, err)
	}
	events, err := s.LoadSessionEvents(worker.ID)
	if err != nil || !slices.ContainsFunc(events, func(e sessionevents.Event) bool {
		return e.Type == "task_queue_retired"
	}) {
		t.Fatalf("retired instruction evidence missing: %+v, %v", events, err)
	}
}
