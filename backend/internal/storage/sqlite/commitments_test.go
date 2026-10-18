package sqlite

import (
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestADeliveryAndADropOfOneCommitmentCannotBothHappen(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bot, err := store.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	asker, err := store.CreateSession(storage.CreateSession{Slug: "asker", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	open := func(request string) int64 {
		t.Helper()
		commitment, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Requester: asker.ID, Request: request})
		if err != nil {
			t.Fatal(err)
		}
		return commitment.ID
	}
	answered, dropped := open("Price the part"), open("Check the stock")
	if err := store.StartSessionTurn(bot.ID, storage.Turn{Output: &storage.TurnOutput{ReplyTo: asker.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnReply(bot.ID, "Four pounds.", []int64{answered}); err != nil {
		t.Fatal(err)
	}
	if err := store.DropCommitment(answered, "no longer needed", time.Now()); !errors.Is(err, storage.ErrCommitmentStaged) {
		t.Fatalf("dropping a commitment the turn's answer delivers: %v", err)
	}
	if err := store.DropCommitment(dropped, "cancelled", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnReply(bot.ID, "In stock.", []int64{dropped}); !errors.Is(err, storage.ErrCommitmentNotOpen) {
		t.Fatalf("an answer took done for a dropped commitment: %v", err)
	}
	answer := storage.TurnDelivery{SessionID: asker.ID, Message: storage.NewInternalQueuedMessage("Four pounds."), Closes: []int64{answered}}
	if err := store.FinishSessionTurn(bot.ID, storage.StatusIdle, "", time.Now(), []storage.TurnDelivery{answer}); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[int64]string{answered: storage.CommitmentDone, dropped: storage.CommitmentDropped} {
		if commitment, err := store.LoadCommitment(id); err != nil || commitment.Status != want {
			t.Fatalf("commitment %d = %+v, %v; want %s", id, commitment, err, want)
		}
	}
}
