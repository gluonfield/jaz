package server

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/sessionlock"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

func TestQueueDrainRetiresObsoleteWakeAndReachesUnrelatedInternalDelivery(t *testing.T) {
	for _, outcome := range []string{"human wait", "completion", "worker started", "restored wake"} {
		t.Run(outcome, func(t *testing.T) {
			store, err := sqlitestore.New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			bot, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot, RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: acp.AgentCodex}})
			if err != nil {
				t.Fatal(err)
			}
			task, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Check the available route"})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.QueueTurn(storage.TurnDelivery{SessionID: bot.ID, Message: storage.NewInternalQueuedMessage("Obsolete owed wake"), Wake: true}); err != nil {
				t.Fatal(err)
			}
			queued, err := store.LoadSession(bot.ID)
			if err != nil || len(queued.QueuedMessages) != 1 {
				t.Fatalf("initial owed wake missing: %+v, %v", queued, err)
			}
			wake := queued.QueuedMessages[0]
			manager := &fakeACPManager{}
			srv := &Server{Store: store, ACP: manager, Locks: sessionlock.New()}
			switch outcome {
			case "human wait":
				_, err = store.AskCommitment(task, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Which input should I use?"}})
			case "completion":
				_, err = store.DeliverMessage(sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "The route is verified."}}, []int64{task.ID})
			case "worker started":
				var worker storage.Session
				worker, err = store.CreateSession(storage.CreateSession{SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: task.ID})
				if err == nil {
					err = store.StartSessionTurn(worker.ID, storage.Turn{})
				}
			case "restored wake":
				if err = store.ClaimQueuedTurn(queued, wake); err == nil {
					_, err = store.AskCommitment(task, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Which input should I use?"}})
				}
				if err == nil {
					srv.restoreQueuedPrompt(bot.ID, wake, 0, "A delayed start failed")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			unrelated := storage.NewInternalQueuedMessage("Unrelated internal delivery")
			if err := store.QueueTurn(storage.TurnDelivery{SessionID: bot.ID, Message: unrelated}); err != nil {
				t.Fatal(err)
			}
			srv.drainQueuedPrompt(context.Background(), bot.ID)
			manager.mu.Lock()
			calls, message := manager.internalCalls, manager.internal.Message
			manager.mu.Unlock()
			if calls != 1 || message != unrelated.Text {
				t.Fatalf("obsolete wake blocked or reached the provider: %d calls, %q", calls, message)
			}
			current, err := store.LoadSession(bot.ID)
			if err != nil || current.Turn == nil || current.Turn.PendingMessage.Text != unrelated.Text || len(current.QueuedMessages) != 0 {
				t.Fatalf("unrelated delivery not claimed once: %+v, %v", current, err)
			}
			events, err := store.LoadSessionEvents(bot.ID)
			if err != nil {
				t.Fatal(err)
			}
			retired := 0
			for _, event := range events {
				if event.Type != "task_queue_retired" {
					continue
				}
				var evidence struct {
					Messages []storage.QueuedMessage `json:"messages"`
				}
				if err := json.Unmarshal([]byte(event.Content), &evidence); err != nil {
					t.Fatal(err)
				}
				for _, message := range evidence.Messages {
					if message.ID == wake.ID && message.Kind == storage.QueuedMessageKindTaskWake {
						retired++
					}
				}
			}
			if retired != 1 {
				t.Fatalf("obsolete wake retained %d times, want once", retired)
			}
		})
	}
}

func TestQueueDrainRetiresRestoredStoppedResultWithoutBlockingAnotherDelivery(t *testing.T) {
	store, err := sqlitestore.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	bot, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBot, RuntimeRef: &storage.RuntimeRef{Type: storage.RuntimeACP, Agent: acp.AgentCodex}})
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Check the available route"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := store.CreateSession(storage.CreateSession{SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TransitionCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, Status: storage.CommitmentStopped}, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "The available routes cannot handle this input; no useful retry condition is known."}}); err != nil {
		t.Fatal(err)
	}
	unrelated := storage.NewInternalQueuedMessage("Independent internal delivery")
	if err := store.QueueTurn(storage.TurnDelivery{SessionID: bot.ID, Message: unrelated}); err != nil {
		t.Fatal(err)
	}
	manager := &fakeACPManager{}
	srv := &Server{Store: store, ACP: manager, Locks: sessionlock.New()}
	old := storage.NewInternalQueuedMessage("Obsolete stopped result")
	old.From = worker.ID
	srv.restoreQueuedPrompt(bot.ID, old, 0, "A delayed start failed")
	srv.drainQueuedPrompt(context.Background(), bot.ID)
	manager.mu.Lock()
	calls, message := manager.internalCalls, manager.internal.Message
	manager.mu.Unlock()
	if calls != 1 || message != unrelated.Text {
		t.Fatalf("stopped result blocked unrelated delivery: %d calls, %q", calls, message)
	}
	loaded, err := store.LoadCommitment(task.ID)
	if err != nil || loaded.Status != storage.CommitmentStopped {
		t.Fatalf("retired result changed dormant task: %+v, %v", loaded, err)
	}
	events, err := store.LoadSessionEvents(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	retained := false
	for _, event := range events {
		retained = retained || event.Type == "task_queue_retired" && strings.Contains(event.Content, old.Text)
	}
	if !retained {
		t.Fatal("restored obsolete result lost its evidence")
	}
}
