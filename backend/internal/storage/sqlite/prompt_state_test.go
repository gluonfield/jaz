package sqlite_test

import (
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestPromptConfirmationCannotRegressOrConfirmALaterTurn(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "prompt-state", RuntimeRef: &storage.RuntimeRef{SessionID: "native"}})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().UTC()
	turn := storage.Turn{StartedAt: started, Prompt: &storage.TurnPrompt{Message: "first task"}}
	if err := store.StartSessionTurn(session.ID, turn); err != nil {
		t.Fatal(err)
	}
	for _, state := range []storage.PromptState{storage.PromptSending, storage.PromptConfirmed, storage.PromptSending} {
		if err := store.SetTurnPromptState(session.ID, "native", started, state); err != nil {
			t.Fatal(err)
		}
	}
	thread, err := store.LoadSession(session.ID)
	if err != nil || thread.Turn.Prompt.State != storage.PromptConfirmed || thread.Turn.Prompt.Message != "first task" {
		t.Fatalf("prompt state regressed or changed input: %+v, %v", thread.Turn, err)
	}
	turn.StartedAt = started.Add(time.Second)
	turn.Prompt = &storage.TurnPrompt{Message: "next task"}
	if err := store.StartSessionTurn(session.ID, turn); err != nil {
		t.Fatal(err)
	}
	if err := store.SetTurnPromptState(session.ID, "native", started, storage.PromptConfirmed); err == nil {
		t.Fatal("an old turn's output confirmed the next task")
	}
	thread, err = store.LoadSession(session.ID)
	if err != nil || thread.Turn.Prompt.State != "" || thread.Turn.Prompt.Message != "next task" {
		t.Fatalf("old confirmation changed the next task: %+v, %v", thread.Turn, err)
	}
}
