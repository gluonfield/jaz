package sqlite

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
	"github.com/wins/jaz/backend/internal/storage/sqlite/generated/botdb"
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

func TestACommitmentListsItsThreadsInTheOrderTheyWereStarted(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bot, err := store.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	commitment, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Ship it"})
	if err != nil {
		t.Fatal(err)
	}
	// Thread ids share a second and differ in a random suffix, so the later
	// thread can sort first.
	for _, id := range []string{"20261019T000000-ffffffff", "20261019T000000-00000000"} {
		if err := store.SaveSession(storage.Session{ID: id, Slug: id, SourceType: storage.SourceBotWorker, SourceID: bot.ID}); err != nil {
			t.Fatal(err)
		}
		if _, err := botdb.New(store.db).LinkCommitmentThread(t.Context(), botdb.LinkCommitmentThreadParams{ThreadID: id, CommitmentID: commitment.ID, BotID: bot.ID}); err != nil {
			t.Fatal(err)
		}
	}
	loaded, err := store.LoadCommitment(commitment.ID)
	if err != nil || len(loaded.Threads) != 2 || loaded.Threads[0].ID != "20261019T000000-ffffffff" {
		t.Fatalf("threads = %+v, %v; want the first started first", loaded.Threads, err)
	}
}

func TestDeliveringACommitmentDropsTheResultsItsThreadsQueuedForTheBot(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bot, err := store.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	thread := func(request string) (int64, string) {
		t.Helper()
		commitment, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Requester: bot.ID, Request: request})
		if err != nil {
			t.Fatal(err)
		}
		worker, err := store.CreateSession(storage.CreateSession{Slug: request, ParentID: bot.ID, SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: commitment.ID})
		if err != nil {
			t.Fatal(err)
		}
		return commitment.ID, worker.ID
	}
	delivered, quotes := thread("quotes")
	dropped, stock := thread("stock")
	result := func(from, text string) storage.QueuedMessage {
		message := storage.NewInternalQueuedMessage(text)
		message.From = from
		return message
	}
	for _, message := range []storage.QueuedMessage{
		storage.NewQueuedMessage("How are the quotes going?", nil),
		result(quotes, "quotes finished"),
		{Text: "check the inbox", Kind: storage.QueuedMessageKindInternal, Output: &storage.TurnOutput{Run: "run-1"}},
		result(stock, "stock finished"),
	} {
		if err := store.QueueTurn(storage.TurnDelivery{SessionID: bot.ID, Message: message}); err != nil {
			t.Fatal(err)
		}
	}

	answer := storage.TurnDelivery{SessionID: bot.ID, Message: storage.NewInternalQueuedMessage("The quotes are in."), Closes: []int64{delivered}}
	if err := store.QueueTurn(answer); err != nil {
		t.Fatal(err)
	}
	// A drop delivers nothing, so its thread's result still wakes the bot.
	if err := store.DropCommitment(dropped, "no longer needed", time.Now()); err != nil {
		t.Fatal(err)
	}
	session, err := store.LoadSession(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, message := range session.QueuedMessages {
		texts = append(texts, message.Text)
	}
	want := []string{"How are the quotes going?", "check the inbox", "stock finished", "The quotes are in."}
	if !slices.Equal(texts, want) {
		t.Fatalf("bot queue = %q, want %q: only the delivered commitment's result goes", texts, want)
	}
}

func TestUnfinishedTasksRetryWithoutDuplicateWakesAndKeepBackoffAcrossRestart(t *testing.T) {
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
	if _, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Compare the quotes"}); err != nil {
		t.Fatal(err)
	}
	wake := func(want bool) {
		t.Helper()
		err := store.QueueTurn(storage.TurnDelivery{SessionID: bot.ID, Message: storage.NewInternalQueuedMessage("[owed]"), Wake: true})
		if want && err != nil || !want && !errors.Is(err, storage.ErrNothingStalled) {
			t.Fatalf("wake = %v, want eligible %v", err, want)
		}
	}
	finish := func() {
		t.Helper()
		session, err := store.LoadSession(bot.ID)
		if err != nil || len(session.QueuedMessages) != 1 {
			t.Fatalf("expected exactly one durable wake: %+v, %v", session, err)
		}
		message := session.QueuedMessages[0]
		session.QueuedMessages = nil
		if err := store.ClaimQueuedTurn(session, message); err != nil {
			t.Fatal(err)
		}
		if err := store.FinishSessionTurn(bot.ID, storage.StatusIdle, "", time.Now(), nil); err != nil {
			t.Fatal(err)
		}
	}
	wake(true)
	wake(false)
	finish()
	wake(false)
	var first int64
	if err := store.db.QueryRow("SELECT wake_at_ms FROM bot_commitments").Scan(&first); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	wake(false)
	if _, err := store.db.Exec("UPDATE bot_commitments SET wake_at_ms = 0"); err != nil {
		t.Fatal(err)
	}
	eligible, err := store.ListWatchBots()
	if err != nil || !slices.Equal(eligible, []string{bot.ID}) {
		t.Fatalf("eligible bots = %v, %v", eligible, err)
	}
	wake(true)
	var second int64
	if err := store.db.QueryRow("SELECT wake_at_ms FROM bot_commitments").Scan(&second); err != nil || second <= first {
		t.Fatalf("retry did not back off: first %d, second %d, error %v", first, second, err)
	}
	wake(false)
	finish()
}

func TestWaitingTaskRequiresADeliveredQuestionAndARealUserReplyReleasesIt(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bot, err := store.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(bot.ID, storage.Message{Role: "user", Content: "Compare the suppliers", FromUser: true}); err != nil {
		t.Fatal(err)
	}
	task, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Compare the suppliers"})
	if err != nil {
		t.Fatal(err)
	}
	task.Status = storage.CommitmentWaitingForUser
	if err := store.UpdateCommitment(task); err == nil {
		t.Fatal("a wait without a delivered question was accepted")
	}
	task.QuestionSeq = 999
	if err := store.UpdateCommitment(task); err == nil {
		t.Fatal("a fabricated question reference was accepted")
	}
	event, err := store.AskCommitment(task, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage,
		RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Which supplier should I compare?"}})
	if err != nil {
		t.Fatal(err)
	}
	task, err = store.LoadCommitment(task.ID)
	if err != nil || task.Status != storage.CommitmentWaitingForUser || task.QuestionSeq != event.Seq || event.Seq == 0 {
		t.Fatalf("question wait = %+v, %v", task, err)
	}
	if err := store.UpdateCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, Note: "Use the user's choice"}); err != nil {
		t.Fatal(err)
	}
	task, err = store.LoadCommitment(task.ID)
	if err != nil || task.Status != storage.CommitmentWaitingForUser || task.QuestionSeq != event.Seq || task.Note != "Use the user's choice" {
		t.Fatalf("progress update cleared the unanswered question: %+v, %v", task, err)
	}
	eligible, err := store.ListWatchBots()
	if err != nil || len(eligible) != 0 {
		t.Fatalf("a genuine user wait woke the bot: %v, %v", eligible, err)
	}
	messages, err := store.LoadMessages(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMessages(bot.ID, messages); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadCommitment(task.ID)
	if err != nil || loaded.Status != storage.CommitmentWaitingForUser {
		t.Fatalf("replaying old user messages released the wait: %+v, %v", loaded, err)
	}
	if err := store.AppendMessageRecords(bot.ID, storage.Message{Role: "user", Content: "Internal report", FromUser: false}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadCommitment(task.ID)
	if err != nil || loaded.Status != storage.CommitmentWaitingForUser {
		t.Fatalf("an internal message released the user wait: %+v, %v", loaded, err)
	}
	if err := store.AppendMessageRecords(bot.ID, storage.Message{Role: "user", Content: "Supplier A and B", FromUser: true}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadCommitment(task.ID)
	if err != nil || loaded.Status != storage.CommitmentPending || loaded.QuestionSeq != 0 {
		t.Fatalf("user reply did not release the wait: %+v, %v", loaded, err)
	}
	if err := store.UpdateCommitment(task); err == nil {
		t.Fatal("a stale update reinstated an answered question")
	}
}

func TestTaskDependenciesBlockExecutionAndRejectCyclesOrOtherBots(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	bot, err := store.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateSession(storage.CreateSession{Slug: "other", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	open := func(bot, request string) storage.Commitment {
		t.Helper()
		task, err := store.OpenCommitment(storage.Commitment{BotID: bot, Request: request})
		if err != nil {
			t.Fatal(err)
		}
		return task
	}
	prerequisite := open(bot.ID, "Read drawings")
	dependent := open(bot.ID, "Cost the part")
	foreign := open(other.ID, "Other bot's work")
	dependent.DependsOn = []int64{prerequisite.ID}
	if err := store.UpdateCommitment(dependent); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateSession(storage.CreateSession{Slug: "too early", SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: dependent.ID}); err == nil {
		t.Fatal("a worker started before its prerequisite ended")
	}
	prerequisite.DependsOn = []int64{dependent.ID}
	if err := store.UpdateCommitment(prerequisite); err == nil {
		t.Fatal("a dependency cycle was accepted")
	}
	dependent.DependsOn = []int64{foreign.ID}
	if err := store.UpdateCommitment(dependent); err == nil {
		t.Fatal("another bot's task was accepted as a prerequisite")
	}
	loaded, err := store.LoadCommitment(dependent.ID)
	if err != nil || !slices.Equal(loaded.DependsOn, []int64{prerequisite.ID}) {
		t.Fatalf("failed dependency update was not atomic: %+v, %v", loaded, err)
	}
	if _, err := store.DeliverMessage(sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage,
		RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Drawings reviewed."}}, []int64{prerequisite.ID}); err != nil {
		t.Fatal(err)
	}
	worker, err := store.CreateSession(storage.CreateSession{Slug: "ready", SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: dependent.ID})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadCommitment(dependent.ID)
	if err != nil || loaded.Status != storage.CommitmentInProgress || len(loaded.Threads) != 1 || loaded.Threads[0].ID != worker.ID {
		t.Fatalf("dependency-ready task did not start: %+v, %v", loaded, err)
	}
	if err := store.StartSessionTurn(worker.ID, storage.Turn{}); err != nil {
		t.Fatal(err)
	}
	late := open(bot.ID, "New prerequisite")
	loaded.DependsOn = []int64{late.ID}
	if err := store.UpdateCommitment(loaded); err == nil {
		t.Fatal("an unfinished prerequisite was added behind an active worker")
	}
	loaded, err = store.LoadCommitment(dependent.ID)
	if err != nil || !loaded.Moving || loaded.Blocked || !slices.Equal(loaded.DependsOn, []int64{prerequisite.ID}) {
		t.Fatalf("late dependency rejection was not atomic: %+v, %v", loaded, err)
	}
	loaded.Status = storage.CommitmentPending
	if err := store.UpdateCommitment(loaded); err == nil {
		t.Fatal("an active task was reset to pending")
	}
}

func TestTaskQuestionSurvivesHistoryReplacementAndReleasesOnTheNextUserMessage(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	bot, err := store.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendMessageRecords(bot.ID,
		storage.Message{Role: "user", Content: "Compare suppliers", FromUser: true},
		storage.Message{Role: "assistant", Content: "Checking the records"},
		storage.Message{Role: "user", Content: "Use the current list", FromUser: true},
	); err != nil {
		t.Fatal(err)
	}
	task, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Compare suppliers"})
	if err != nil {
		t.Fatal(err)
	}
	updates := make(chan error, 2)
	go func() {
		_, err := store.AskCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID}, sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage,
			RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "Which supplier should I use?"}})
		updates <- err
	}()
	go func() {
		updates <- store.UpdateCommitment(storage.Commitment{ID: task.ID, BotID: bot.ID, Note: "Use the user's answer"})
	}()
	for range 2 {
		if err := <-updates; err != nil {
			t.Fatalf("parallel task updates: %v", err)
		}
	}
	task, err = store.LoadCommitment(task.ID)
	if err != nil || task.Note != "Use the user's answer" || task.Status != storage.CommitmentWaitingForUser || task.Request != "Compare suppliers" {
		t.Fatalf("parallel task update lost a field or question: %+v, %v", task, err)
	}
	if err := store.SaveMessages(bot.ID, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadCommitment(task.ID)
	if err != nil || loaded.Status != storage.CommitmentWaitingForUser {
		t.Fatalf("history replacement released an unanswered question: %+v, %v", loaded, err)
	}
	if err := store.AppendMessageRecords(bot.ID, storage.Message{Role: "user", Content: "Supplier A", FromUser: true}); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.LoadCommitment(task.ID)
	if err != nil || loaded.Status != storage.CommitmentPending {
		t.Fatalf("renumbered user message did not release the question: %+v, %v", loaded, err)
	}
	if err := store.UpdateCommitment(task); err == nil {
		t.Fatal("a stale question update reinstated the wait after history replacement")
	}
}

func TestTaskRecoveryAndCompletionAccountForWorkersStartedByItsWorker(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	bot, err := store.CreateSession(storage.CreateSession{Slug: "bot", SourceType: storage.SourceBot})
	if err != nil {
		t.Fatal(err)
	}
	task, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Review the suppliers"})
	if err != nil {
		t.Fatal(err)
	}
	worker, err := store.CreateSession(storage.CreateSession{Slug: "worker", SourceType: storage.SourceBotWorker, SourceID: bot.ID, ParentID: bot.ID, Commitment: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateSession(storage.CreateSession{Slug: "review", SourceType: storage.SourceBotWorker, SourceID: bot.ID, ParentID: worker.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartSessionTurn(child.ID, storage.Turn{}); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.LoadCommitment(task.ID)
	if err != nil || !loaded.Moving {
		t.Fatalf("task ignored its active descendant worker: %+v, %v", loaded, err)
	}
	if len(loaded.Threads) != 1 || loaded.Threads[0].ID != worker.ID {
		t.Fatalf("derived worker activity changed the direct task links: %+v", loaded.Threads)
	}
	watch, err := store.ListWatchBots()
	if err != nil || len(watch) != 0 {
		t.Fatalf("active descendant triggered recovery: %v, %v", watch, err)
	}
	if err := store.DropCommitment(task.ID, "Cancelled", time.Now()); !errors.Is(err, storage.ErrCommitmentWorking) {
		t.Fatalf("task closed while its descendant worked: %v", err)
	}
	if _, err := store.DeliverMessage(sessionevents.Event{SessionID: bot.ID, Type: sessionevents.TypeRoomMessage,
		RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", Text: "This must not be delivered"}}, []int64{task.ID}); !errors.Is(err, storage.ErrCommitmentWorking) {
		t.Fatalf("task completed while its descendant worked: %v", err)
	}
	events, err := store.LoadSessionEvents(bot.ID)
	if err != nil || len(events) != 0 {
		t.Fatalf("refused completion leaked its result: %+v, %v", events, err)
	}
	if err := store.FinishSessionTurn(child.ID, storage.StatusIdle, "", time.Now(), nil); err != nil {
		t.Fatal(err)
	}
	watch, err = store.ListWatchBots()
	if err != nil || !slices.Equal(watch, []string{bot.ID}) {
		t.Fatalf("finished descendant did not release recovery: %v, %v", watch, err)
	}
	if err := store.DropCommitment(task.ID, "Cancelled", time.Now()); err != nil {
		t.Fatal(err)
	}
}
