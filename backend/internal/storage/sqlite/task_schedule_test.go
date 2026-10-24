package sqlite

import (
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
)

func TestTaskChecksWaitUntilDueAcrossProgressAndRestart(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bot, err := store.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Check the supplier reply"})
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().UTC().Add(time.Hour).Truncate(time.Millisecond)
	if err := store.UpdateCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, CheckAt: &future}); err != nil {
		t.Fatal(err)
	}
	// Changed worker progress must not bypass the explicitly scheduled check.
	if _, err := store.CreateSession(storage.CreateSession{Slug: "worker", SourceType: storage.SourceBotWorker, SourceID: bot.ID, ParentID: bot.ID, Commitment: task.ID}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadCommitment(task.ID)
	if err != nil || loaded.CheckAt == nil || !loaded.CheckAt.Equal(future) {
		t.Fatalf("restart lost the scheduled check: %+v, %v", loaded, err)
	}
	if bots, err := store.ListWatchBots(); err != nil || len(bots) != 0 {
		t.Fatalf("future check became eligible: %v, %v", bots, err)
	}
	wake := storage.TurnDelivery{SessionID: bot.ID, Message: storage.NewInternalQueuedMessage("Check unfinished work"), Wake: true}
	if err := store.QueueTurn(wake); !errors.Is(err, storage.ErrNothingStalled) {
		t.Fatalf("queue bypassed the due time: %v", err)
	}
	past := time.Now().UTC().Add(-time.Minute)
	if err := store.UpdateCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, CheckAt: &past}); err != nil {
		t.Fatal(err)
	}
	if bots, err := store.ListWatchBots(); err != nil || len(bots) != 1 || bots[0] != bot.ID {
		t.Fatalf("due task was not eligible: %v, %v", bots, err)
	}
	if err := store.QueueTurn(wake); err != nil {
		t.Fatal(err)
	}
	if err := store.QueueTurn(wake); !errors.Is(err, storage.ErrNothingStalled) {
		t.Fatalf("due task queued duplicate wakes: %v", err)
	}
	thread, err := store.LoadSession(bot.ID)
	if err != nil || len(thread.QueuedMessages) != 1 {
		t.Fatalf("due task queued messages: %+v, %v", thread.QueuedMessages, err)
	}
}
