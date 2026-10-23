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

func TestATurnKeepsWhatItsAgentWasShownOnlyOnceTheAgentTookItsPrompt(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "prompt-digest", RuntimeRef: &storage.RuntimeRef{SessionID: "native"}})
	if err != nil {
		t.Fatal(err)
	}
	turn := func(digest string, states ...storage.PromptState) string {
		t.Helper()
		started := time.Now().UTC()
		if err := store.StartSessionTurn(session.ID, storage.Turn{StartedAt: started, PromptDigest: digest, Prompt: &storage.TurnPrompt{Message: "task"}}); err != nil {
			t.Fatal(err)
		}
		for _, state := range states {
			if err := store.SetTurnPromptState(session.ID, "native", started, state); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.FinishSessionTurn(session.ID, storage.StatusIdle, "", time.Now().UTC(), nil); err != nil {
			t.Fatal(err)
		}
		thread, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		return thread.PromptDigest
	}
	if kept := turn("taken", storage.PromptSending, storage.PromptConfirmed); kept != "taken" {
		t.Fatalf("a turn whose agent took its prompt kept %q", kept)
	}
	// A turn stopped before or while its prompt was sent ends idle too.
	if kept := turn("never sent"); kept != "taken" {
		t.Fatalf("a turn stopped before its prompt was sent kept %q", kept)
	}
	if kept := turn("still sending", storage.PromptSending); kept != "taken" {
		t.Fatalf("a turn whose prompt was not taken kept %q", kept)
	}
}
