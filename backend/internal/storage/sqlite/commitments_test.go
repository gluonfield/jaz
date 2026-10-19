package sqlite

import (
	"errors"
	"slices"
	"testing"
	"time"

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

func TestAStalledCommitmentWakesItsBotOnceThenStallsForTheUserAcrossARestart(t *testing.T) {
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
	commitment, err := store.OpenCommitment(storage.Commitment{BotID: bot.ID, Request: "Compare the quotes", Waiting: storage.WaitingRequester})
	if err != nil {
		t.Fatal(err)
	}
	wakes := func(want bool, why string) {
		t.Helper()
		err := store.QueueTurn(storage.TurnDelivery{SessionID: bot.ID, Message: storage.NewInternalQueuedMessage("[owed]"), Wake: true})
		if want && err != nil || !want && !errors.Is(err, storage.ErrNothingStalled) {
			t.Fatalf("%s: wake = %v, want woken %v", why, err, want)
		}
	}
	stalls := func(want int, why string) {
		t.Helper()
		if stalled, err := store.MarkStalled(bot.ID); err != nil || len(stalled) != want {
			t.Fatalf("%s: stalled = %+v, %v; want %d", why, stalled, err, want)
		}
	}
	// takeTurn runs the bot's queued turn to an end that does nothing.
	takeTurn := func() {
		t.Helper()
		session, err := store.LoadSession(bot.ID)
		if err != nil || len(session.QueuedMessages) == 0 {
			t.Fatalf("bot %+v, %v; want a queued turn", session, err)
		}
		prompt := session.QueuedMessages[0]
		session.QueuedMessages = session.QueuedMessages[1:]
		if err := store.ClaimQueuedTurn(session, prompt); err != nil {
			t.Fatal(err)
		}
		if err := store.FinishSessionTurn(bot.ID, storage.StatusIdle, "", time.Now(), nil); err != nil {
			t.Fatal(err)
		}
	}
	runs := func(thread string) {
		t.Helper()
		if err := store.StartSessionTurn(thread, storage.Turn{}); err != nil {
			t.Fatal(err)
		}
	}
	ends := func(thread string) {
		t.Helper()
		if err := store.FinishSessionTurn(thread, storage.StatusIdle, "", time.Now(), nil); err != nil {
			t.Fatal(err)
		}
	}

	wakes(false, "waiting on its requester")
	thread, err := store.CreateSession(storage.CreateSession{Slug: "quotes", SourceType: storage.SourceBotWorker, SourceID: bot.ID, Commitment: commitment.ID})
	if err != nil {
		t.Fatal(err)
	}
	if loaded, err := store.LoadCommitment(commitment.ID); err != nil || loaded.Waiting != "" {
		t.Fatalf("commitment = %+v, %v; want the thread started for it to end its wait", loaded, err)
	}
	runs(thread.ID)
	wakes(false, "its thread runs")
	ends(thread.ID)
	runs(bot.ID)
	wakes(false, "its bot is taking a turn")
	ends(bot.ID)
	wakes(true, "its thread finished and its bot ended its turn")
	stalls(0, "its bot has not yet taken the turn that wakes it")
	takeTurn()
	wakes(false, "its bot was woken at this progress")
	stalls(1, "nothing moved after its bot was woken")
	stalls(0, "the user was told already")
	waits := func(waiting string) {
		t.Helper()
		loaded, err := store.LoadCommitment(commitment.ID)
		if err != nil {
			t.Fatal(err)
		}
		loaded.Waiting = waiting
		if err := store.UpdateCommitment(loaded); err != nil {
			t.Fatal(err)
		}
	}
	if loaded, err := store.LoadCommitment(commitment.ID); err != nil || !loaded.Stalled {
		t.Fatalf("commitment = %+v, %v; want it stalled", loaded, err)
	}
	waits(storage.WaitingOutside)
	if loaded, err := store.LoadCommitment(commitment.ID); err != nil || loaded.Stalled {
		t.Fatalf("commitment = %+v, %v; want a declared wait to show instead of the stall", loaded, err)
	}
	waits("")

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if store, err = New(root); err != nil {
		t.Fatal(err)
	}
	wakes(false, "after a restart, its bot was woken at this progress")
	stalls(0, "after a restart, the user was told already")

	stalled := func(want bool, why string) {
		t.Helper()
		if loaded, err := store.LoadCommitment(commitment.ID); err != nil || loaded.Stalled != want {
			t.Fatalf("%s: commitment = %+v, %v; want stalled %v", why, loaded, err, want)
		}
	}
	runs(thread.ID)
	stalled(false, "its thread runs again")
	ends(thread.ID)
	stalled(false, "its thread's new turn ended")
	wakes(true, "its thread ran again")
	takeTurn()
	waits(storage.WaitingRequester)
	stalls(0, "waiting on its requester")
	waits("")
	wakes(true, "the wait it declared ended the stall it was woken about")

	takeTurn()
	runs(thread.ID)
	ends(thread.ID)
	runs(thread.ID)
	if err := store.UpdateSessionStatus(thread.ID, storage.StatusInterrupted, "", time.Time{}); err != nil {
		t.Fatal(err)
	}
	wakes(false, "recovery resumes or fails its interrupted thread")
	for _, why := range []string{"its thread failed", "its thread failed again"} {
		if err := store.FinishSessionTurn(thread.ID, storage.StatusError, "boom", time.Now(), nil); err != nil {
			t.Fatal(err)
		}
		wakes(true, why)
		takeTurn()
		runs(thread.ID)
	}
}
