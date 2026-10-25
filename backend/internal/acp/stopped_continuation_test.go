package acp_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/server"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/sessionrecovery"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestStoppedContinuationRemainsDormantAcrossRecovery(t *testing.T) {
	for _, state := range []storage.PromptState{"", storage.PromptSending, storage.PromptConfirmed} {
		t.Run(string(state), func(t *testing.T) {
			store, root := newTestStore(t)
			bot, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot})
			if err != nil {
				t.Fatal(err)
			}
			task, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Find a supported route"})
			if err != nil {
				t.Fatal(err)
			}
			owner, err := store.CreateSession(storage.CreateSession{
				ParentID: bot.ID, SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: task.ID,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := store.TransitionCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, Status: storage.CommitmentStopped}, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "No supported route exists. Wait for fresh instructions."}}); err != nil {
				t.Fatal(err)
			}
			origin := time.Now().UTC().Add(-time.Minute)
			worker := storage.Session{
				ID: store.NewSessionID(), Slug: "retained-child", ParentID: owner.ID,
				SourceType: storage.SourceBotWorker, SourceID: bot.ID, Status: storage.StatusInterrupted,
				RuntimeRef: &storage.RuntimeRef{Agent: acp.AgentCodex, SessionID: "native", Cwd: root},
				Turn:       &storage.Turn{StartedAt: time.Now().UTC(), ContinuationOf: origin, Prompt: &storage.TurnPrompt{State: state, Message: "Continue from where you left off."}},
			}
			if err := store.SaveSession(worker); err != nil {
				t.Fatal(err)
			}
			if err := store.QueueTurn(storage.TurnDelivery{SessionID: worker.ID, Message: storage.NewQueuedMessage("Queued instruction", nil)}); err != nil {
				t.Fatal(err)
			}
			requestLog := filepath.Join(root, "requests.jsonl")
			manager := newFakeNamedAgentManagerWithOptions(t, store, root, acp.AgentCodex, map[string]string{"JAZ_FAKE_ACP_REQUEST_LOG": requestLog}, "", "")
			t.Cleanup(manager.Close)
			locks, events := sessionlock.New(), sessionevents.New()
			handler := &server.Server{Store: store, ACP: manager, Locks: locks, Events: events}
			for range 2 {
				if err := sessionrecovery.Resume(t.Context(), store, handler, locks, events, log.New(io.Discard)); err != nil {
					t.Fatal(err)
				}
			}
			if err := handler.ResumeInterruptedTurn(t.Context(), worker.ID); !errors.Is(err, storage.ErrTaskStopped) {
				t.Fatalf("server admitted a stopped continuation: %v", err)
			}
			if err := manager.ResumeInterruptedTurn(t.Context(), worker.ID); !errors.Is(err, storage.ErrTaskStopped) {
				t.Fatalf("runtime admitted a stopped continuation: %v", err)
			}
			current, err := store.LoadSession(worker.ID)
			if err != nil || current.Status != storage.StatusInterrupted || current.Turn == nil || !current.Turn.ContinuationOf.Equal(origin) || current.Turn.Prompt.State != state || len(current.QueuedMessages) != 1 {
				t.Fatalf("recovery altered dormant work: %+v %v", current, err)
			}
			if _, err := os.Stat(requestLog); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("stopped work started a provider process: %v", err)
			}
		})
	}
}
