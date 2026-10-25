package sqlite

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestStoppedTaskPersistsDormancyAndRetiresOnlyItsOldIntentOnResume(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = s.Close()
	})
	bot, err := s.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Find a supported route"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := s.CreateSession(storage.CreateSession{Slug: "worker", ParentID: bot.ID, SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	child, err := s.CreateSession(storage.CreateSession{Slug: "child", ParentID: worker.ID, SourceType: storage.SourceBotWorker, SourceID: bot.ID})
	if err != nil {
		t.Fatal(err)
	}
	dependent, err := s.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Use the route"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateCommitment(storage.Commitment{ID: dependent.ID, BotID: bot.ID, DependsOn: []int64{task.ID}}); err != nil {
		t.Fatal(err)
	}
	transition := func(status, reason string) error {
		t.Helper()
		_, err := s.TransitionCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, Status: status}, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: reason}})
		return err
	}
	queue := func(id string, message storage.QueuedMessage) {
		t.Helper()
		if err := s.QueueTurn(storage.TurnDelivery{SessionID: id, Message: message}); err != nil {
			t.Fatal(err)
		}
	}
	old := storage.NewInternalQueuedMessage("Earlier worker result")
	old.From = child.ID
	queue(bot.ID, old)
	queue(bot.ID, storage.NewQueuedMessage("Unrelated user work", nil))
	queue(child.ID, storage.QueuedMessage{Text: "Old task instruction", From: worker.ID})
	queue(child.ID, storage.QueuedMessage{Text: "Queued user direction", SentAt: time.Now()})
	unrelatedSender, err := s.CreateSession(storage.CreateSession{Slug: "separate-sender"})
	if err != nil {
		t.Fatal(err)
	}
	queue(child.ID, storage.QueuedMessage{Text: "Separate queued request", From: unrelatedSender.ID})
	reason := "Checked both documented routes; neither supports this input. Retry when an implementation supporting it is available."
	if err := transition(storage.CommitmentStopped, " "); err == nil {
		t.Fatal("blank stop reason accepted")
	}
	if err := transition(storage.CommitmentStopped, reason); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadCommitment(task.ID)
	if err != nil || loaded.Status != storage.CommitmentStopped || loaded.Note != reason || loaded.Request != task.Request || loaded.Moving || loaded.Stalled || !loaded.ClosedAt.IsZero() {
		t.Fatalf("persisted stopped task = %+v, %v", loaded, err)
	}
	open, err := s.ListOpenCommitments(bot.ID)
	if err != nil || len(open) != 2 || !open[1].Blocked {
		t.Fatalf("unfinished/dependency state = %+v, %v", open, err)
	}
	botStateBeforeWake, err := s.LoadSession(bot.ID)
	if err != nil || len(botStateBeforeWake.QueuedMessages) != 1 {
		t.Fatalf("unrelated user queue was lost: %+v, %v", botStateBeforeWake, err)
	}
	userPrompt := botStateBeforeWake.QueuedMessages[0]
	botStateBeforeWake.QueuedMessages = nil
	if err := s.ClaimQueuedTurn(botStateBeforeWake, userPrompt); err != nil {
		t.Fatal(err)
	}
	if err := s.FinishSessionTurn(bot.ID, storage.StatusIdle, "", time.Now(), nil); err != nil {
		t.Fatal(err)
	}
	if bots, err := s.ListWatchBots(); err != nil || len(bots) != 0 {
		t.Fatalf("stopped task woke bot: %v, %v", bots, err)
	}
	if err := s.QueueTurn(storage.TurnDelivery{SessionID: bot.ID, Message: storage.NewInternalQueuedMessage("owed"), Wake: true}); !errors.Is(err, storage.ErrNothingStalled) {
		t.Fatalf("stopped wake = %v", err)
	}
	for _, id := range []string{worker.ID, child.ID, child.Slug} {
		if err := s.CheckTaskAdmission(id); !errors.Is(err, storage.ErrTaskStopped) {
			t.Fatalf("admitted stopped owner %s: %v", id, err)
		}
	}
	if err := s.StartSessionTurn(child.ID, storage.Turn{}); err == nil {
		t.Fatal("foreground admission bypassed dormancy")
	}
	if _, err := s.ClaimTurn(child.ID, storage.Turn{}); err == nil {
		t.Fatal("autonomous admission bypassed dormancy")
	}
	if err := s.SetBackgroundWork(child.ID, true); err == nil {
		t.Fatal("background admission bypassed dormancy")
	}
	if _, err := s.CreateSession(storage.CreateSession{Slug: "descendant", ParentID: child.ID, SourceType: storage.SourceBotWorker, SourceID: bot.ID}); !errors.Is(err, storage.ErrTaskStopped) {
		t.Fatalf("new descendant = %v", err)
	}
	if _, err := s.CreateSession(storage.CreateSession{Slug: "direct", SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: task.ID}); err == nil {
		t.Fatal("new direct worker bypassed dormancy")
	}
	for _, status := range []string{storage.CommitmentInProgress, storage.CommitmentPending, storage.CommitmentWaitingForUser} {
		if err := s.UpdateCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, Status: status}); err == nil {
			t.Fatalf("ordinary %s update resumed task", status)
		}
	}
	if err := s.StartSessionTurn(bot.ID, storage.Turn{Output: &storage.TurnOutput{ReplyTo: ""}}); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendTurnReply(bot.ID, "A stale success", []int64{task.ID}); !errors.Is(err, storage.ErrTaskStopped) {
		t.Fatalf("staged completion = %v", err)
	}
	if _, err := s.DeliverMessage(sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "A stale success"}}, []int64{task.ID}); !errors.Is(err, storage.ErrTaskStopped) {
		t.Fatalf("direct completion = %v", err)
	}
	if err := s.QueueTurn(storage.TurnDelivery{SessionID: bot.ID, Message: storage.NewInternalQueuedMessage("A stale success"), Closes: []int64{task.ID}}); !errors.Is(err, storage.ErrTaskStopped) {
		t.Fatalf("queued completion = %v", err)
	}
	if err := s.FinishSessionTurn(bot.ID, storage.StatusIdle, "", time.Now(), []storage.TurnDelivery{{SessionID: bot.ID, Message: storage.NewInternalQueuedMessage("A stale success"), Closes: []int64{task.ID}}}); err != nil {
		t.Fatal(err)
	}
	queue(bot.ID, old)
	queue(bot.ID, storage.NewQueuedMessage("Unrelated user work", nil))
	botState, err := s.LoadSession(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(botState.QueuedMessages) != 1 || botState.QueuedMessages[0].Text != "Unrelated user work" {
		t.Fatalf("stale results or unrelated queue changed: %+v", botState.QueuedMessages)
	}
	childState, err := s.LoadSession(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ClaimQueuedTurn(childState, childState.QueuedMessages[0]); !errors.Is(err, storage.ErrTaskStopped) {
		t.Fatalf("queued worker admitted = %v", err)
	}
	if err := transition(storage.CommitmentPending, ""); err == nil {
		t.Fatal("resume accepted without justification")
	}
	if err := transition(storage.CommitmentPending, "The user supplied the newly supported input; review the new route."); err != nil {
		t.Fatal(err)
	}
	childState, err = s.LoadSession(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(childState.QueuedMessages) != 2 || childState.QueuedMessages[0].Text != "Queued user direction" || childState.QueuedMessages[1].Text != "Separate queued request" {
		t.Fatalf("resume replayed stale intent or lost user work: %+v", childState.QueuedMessages)
	}
	evidence, err := s.LoadSessionEvents(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(evidence, func(e sessionevents.Event) bool {
		return e.Type == "task_queue_retired" && strings.Contains(e.Content, "Old task instruction")
	}) {
		t.Fatal("retired intent lost its evidence")
	}
	chat, err := s.LoadSessionEvents(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(chat, func(e sessionevents.Event) bool {
		return e.RoomMessage != nil && e.RoomMessage.Text == reason
	}) {
		t.Fatal("resume lost the stop explanation")
	}
	if err := s.StartSessionTurn(child.ID, storage.Turn{}); err != nil {
		t.Fatalf("fresh reviewed work cannot start: %v", err)
	}
}

func TestStoppedTaskTransitionRefusesRawOwnedLivenessAndStagedDelivery(t *testing.T) {
	for _, state := range []string{"running", "interrupted", "background", "archived-running", "archived-background", "staged"} {
		t.Run(state, func(t *testing.T) {
			s, err := New(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			bot, err := s.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
			if err != nil {
				t.Fatal(err)
			}
			task, err := s.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Find a route"})
			if err != nil {
				t.Fatal(err)
			}
			worker, err := s.CreateSession(storage.CreateSession{Slug: "worker", SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: task.ID})
			if err != nil {
				t.Fatal(err)
			}
			child, err := s.CreateSession(storage.CreateSession{Slug: "child", ParentID: worker.ID, SourceType: storage.SourceBotWorker, SourceID: bot.ID})
			if err != nil {
				t.Fatal(err)
			}
			switch state {
			case "running", "archived-running":
				err = s.StartSessionTurn(child.ID, storage.Turn{})
			case "interrupted":
				err = s.UpdateSessionStatus(child.ID, storage.StatusInterrupted, "", time.Time{})
			case "background", "archived-background":
				err = s.SetBackgroundWork(child.ID, true)
			case "staged":
				err = s.StartSessionTurn(bot.ID, storage.Turn{Output: &storage.TurnOutput{Done: []int64{task.ID}}})
			}
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(state, "archived-") {
				if err := s.SetArchived(child.ID, true); err != nil {
					t.Fatal(err)
				}
			}
			_, err = s.TransitionCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, Status: storage.CommitmentStopped}, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Both routes failed; no useful retry condition is known."}})
			if err == nil {
				t.Fatal("stopped while real work or staged delivery remained")
			}
			stored, err := s.LoadCommitment(task.ID)
			if err != nil {
				t.Fatal(err)
			}
			chat, err := s.LoadSessionEvents(bot.ID)
			if err != nil {
				t.Fatal(err)
			}
			if stored.Status != storage.CommitmentInProgress || len(chat) != 0 {
				t.Fatalf("refusal partially persisted: %+v, %+v", stored, chat)
			}
		})
	}
}

func TestHumanWaitCanStopWithoutAnswerAndCancelWithoutReplayingIntent(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bot, err := s.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Find a route"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := s.CreateSession(storage.CreateSession{Slug: "worker", SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AskCommitment(task, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Which route should I pursue?"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.QueueTurn(storage.TurnDelivery{SessionID: worker.ID, Message: storage.QueuedMessage{Text: "An obsolete plan", From: bot.ID}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TransitionCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, Status: storage.CommitmentStopped}, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Both routes are unsupported, so the question no longer advances the task. No useful retry condition is known."}}); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.LoadCommitment(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.QuestionSeq != 0 || loaded.ReplySeq != 0 || loaded.CheckAt != nil {
		t.Fatalf("stop kept an obsolete question or timer: %+v", loaded)
	}
	if err := s.DropCommitment(task.ID, "The user abandoned it.", time.Now()); err != nil {
		t.Fatal(err)
	}
	state, err := s.LoadSession(worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.QueuedMessages) != 0 {
		t.Fatal("cancellation released obsolete work")
	}
	events, err := s.LoadSessionEvents(worker.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(events, func(e sessionevents.Event) bool {
		return strings.Contains(e.Content, "An obsolete plan")
	}) {
		t.Fatal("cancellation lost the old instruction evidence")
	}
}

func TestStoppedTaskRetiresItsOwedWakeWithoutLosingUnrelatedQueuedOrRunningWork(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	bot, err := s.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	task, err := s.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Find a route"})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.QueueTurn(storage.TurnDelivery{SessionID: bot.ID, Message: storage.NewInternalQueuedMessage("Reconcile owed work"), Wake: true}); err != nil {
		t.Fatal(err)
	}
	for _, message := range []storage.QueuedMessage{storage.NewInternalQueuedMessage("A separate routine"), storage.NewQueuedMessage("Unrelated user work", nil)} {
		if err := s.QueueTurn(storage.TurnDelivery{SessionID: bot.ID, Message: message}); err != nil {
			t.Fatal(err)
		}
	}
	before, err := s.LoadSession(bot.ID)
	if err != nil || len(before.QueuedMessages) != 3 {
		t.Fatalf("initial queue = %+v, %v", before.QueuedMessages, err)
	}
	other, err := s.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Separate analysis"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := s.CreateSession(storage.CreateSession{Slug: "separate", SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: other.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.StartSessionTurn(worker.ID, storage.Turn{}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TransitionCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, Status: storage.CommitmentStopped}, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "The documented routes cannot handle this input. No useful retry condition is known."}}); err != nil {
		t.Fatal(err)
	}
	after, err := s.LoadSession(bot.ID)
	if err != nil || len(after.QueuedMessages) != 2 || after.QueuedMessages[0].Text != "A separate routine" || after.QueuedMessages[1].Text != "Unrelated user work" {
		t.Fatalf("stop damaged unrelated work: %+v, %v", after.QueuedMessages, err)
	}
	unrelated, err := s.LoadSession(worker.ID)
	if err != nil || unrelated.Status != storage.StatusRunning || unrelated.Turn == nil {
		t.Fatalf("unrelated worker was suspended: %+v, %v", unrelated, err)
	}
	staleWake := before.QueuedMessages[0]
	before.QueuedMessages = before.QueuedMessages[:2]
	if err := s.ClaimQueuedTurn(before, staleWake); !errors.Is(err, storage.ErrNothingStalled) {
		t.Fatalf("stale wake admitted = %v", err)
	}
	user := after.QueuedMessages[1]
	before.QueuedMessages = before.QueuedMessages[:2]
	if err := s.ClaimQueuedTurn(before, user); err != nil {
		t.Fatal(err)
	}
	claimed, err := s.LoadSession(bot.ID)
	if err != nil || len(claimed.QueuedMessages) != 1 || claimed.QueuedMessages[0].Text != "A separate routine" {
		t.Fatalf("stale claim replayed the obsolete wake: %+v, %v", claimed.QueuedMessages, err)
	}
	events, err := s.LoadSessionEvents(bot.ID)
	if err != nil || !slices.ContainsFunc(events, func(e sessionevents.Event) bool {
		return e.Type == "task_queue_retired" && strings.Contains(e.Content, "Reconcile owed work")
	}) {
		t.Fatalf("obsolete wake evidence missing: %+v, %v", events, err)
	}
}
