package sqlite

import (
	"errors"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestTurnCompletionAndDeliveriesCommitOnceAndSurviveReopen(t *testing.T) {
	root := t.TempDir()
	store, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := store.CreateSession(storage.CreateSession{Slug: "parent"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateSession(storage.CreateSession{Slug: "child", ParentID: parent.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartSessionTurn(child.ID, storage.Turn{NotifyParent: true}); err != nil {
		t.Fatal(err)
	}
	delivery := storage.TurnDelivery{SessionID: parent.ID, Message: storage.NewInternalQueuedMessage("result")}
	// An answer for a thread that is gone falls back to the finishing thread.
	undelivered := storage.NewInternalQueuedMessage("undelivered answer")
	gone := storage.TurnDelivery{SessionID: "missing", Message: storage.NewInternalQueuedMessage("answer"), Fallback: &undelivered}
	for range 2 {
		if err := store.FinishSessionTurn(child.ID, storage.StatusIdle, "", time.Now(), []storage.TurnDelivery{delivery, gone}); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	storedChild, _ := store.LoadSession(child.ID)
	storedParent, _ := store.LoadSession(parent.ID)
	if storedChild.Status != storage.StatusIdle || storedChild.Turn != nil || len(storedChild.QueuedMessages) != 1 || storedChild.QueuedMessages[0].Text != "undelivered answer" {
		t.Fatalf("reopened child = %+v, want its turn finished once with the undelivered answer queued", storedChild)
	}
	if len(storedParent.QueuedMessages) != 1 || storedParent.QueuedMessages[0].Text != "result" {
		t.Fatalf("reopened parent = %+v, want the result once", storedParent)
	}
	if err := store.ClaimQueuedTurn(storage.Session{ID: parent.ID}, storedParent.QueuedMessages[0]); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = New(root)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := store.LoadSession(parent.ID)
	if err != nil || claimed.Status != storage.StatusInterrupted || len(claimed.QueuedMessages) != 0 || claimed.Turn.PendingMessage.Text != "result" {
		t.Fatalf("interrupted queue claim: %+v, %v", claimed, err)
	}
}

func TestTurnIntentUpdateKeepsRepliesWrittenAfterSnapshot(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "reply-during-steering"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.StartSessionTurn(session.ID, storage.Turn{Output: &storage.TurnOutput{ReplyTo: "sender"}}); err != nil {
		t.Fatal(err)
	}
	before, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AppendTurnReply(session.ID, "Reply after intent was read", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.SetTurnIntent(session.ID, true, before.Turn.ParentVisible, before.Turn.NotifyParent); err != nil {
		t.Fatal(err)
	}
	after, err := store.LoadSession(session.ID)
	if err != nil || !after.Turn.GoalRequested || len(after.Turn.Output.Replies) != 1 {
		t.Fatalf("intent update lost reply: %+v, %v", after.Turn, err)
	}
}

func TestWhatATurnShowsInItsChatMarksItsOutputShown(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "bot"})
	if err != nil {
		t.Fatal(err)
	}
	shown := func(events ...sessionevents.Event) bool {
		t.Helper()
		if err := store.StartSessionTurn(session.ID, storage.Turn{Output: &storage.TurnOutput{Run: "run-1"}}); err != nil {
			t.Fatal(err)
		}
		if err := store.AppendSessionEvents(session.ID, events...); err != nil {
			t.Fatal(err)
		}
		stored, err := store.LoadSession(session.ID)
		if err != nil {
			t.Fatal(err)
		}
		return stored.Turn.Output.Shown
	}
	app := func(presented bool) sessionevents.Event {
		return sessionevents.Event{Type: sessionevents.TypeMCPApp, MCPApp: &sessionevents.MCPAppEvent{ServerID: "tasks", Tool: "show_issues", Presented: presented, Result: []byte(`{}`)}}
	}
	message := func(speaker string) sessionevents.Event {
		return sessionevents.Event{Type: sessionevents.TypeRoomMessage, RoomMessage: &sessionevents.RoomMessageEvent{Speaker: speaker, Name: speaker, Text: "hi"}}
	}
	if shown(app(false), message("user")) {
		t.Fatal("an app result Jaz did not present, or the user's message, marked the turn shown")
	}
	if !shown(app(true)) || !shown(message("bot")) {
		t.Fatal("a presented app or the bot's message left the turn unshown")
	}
	if err := store.StartSessionTurn(session.ID, storage.Turn{}); err != nil {
		t.Fatal(err)
	}
	if err := store.AppendSessionEvents(session.ID, message("bot")); err != nil {
		t.Fatal(err)
	}
	if stored, err := store.LoadSession(session.ID); err != nil || stored.Turn.Output != nil {
		t.Fatalf("a turn without output = %+v, %v", stored.Turn, err)
	}
}

func TestATurnTheAgentBeganHoldsTheThreadAndOwesWhatFinishesDuringIt(t *testing.T) {
	store, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	session, err := store.CreateSession(storage.CreateSession{Slug: "agent-run"})
	if err != nil {
		t.Fatal(err)
	}
	queued := storage.NewInternalQueuedMessage("a message from the bot")
	if started, err := store.ClaimTurn(session.ID, storage.Turn{AllowSilence: true, Output: &storage.TurnOutput{ReplyTo: "bot"}}); err != nil || !started {
		t.Fatalf("start = %v, %v; want the free thread claimed", started, err)
	}
	if started, err := store.ClaimTurn(session.ID, storage.Turn{}); err != nil || started {
		t.Fatalf("second start = %v, %v; want the held thread refused", started, err)
	}
	// A queue claim that read the thread before the run began loses to it.
	if err := store.ClaimQueuedTurn(storage.Session{ID: session.ID, QueuedMessages: []storage.QueuedMessage{queued}}, queued); !errors.Is(err, storage.ErrThreadRunning) {
		t.Fatalf("claim = %v, want ErrThreadRunning", err)
	}
	if err := store.AppendTurnReply(session.ID, "The suite passed.", nil); err != nil {
		t.Fatal(err)
	}
	if err := store.OweTurn(session.ID, storage.Turn{ParentVisible: true, NotifyParent: true, ParentOutput: &storage.TurnOutput{ReplyTo: "asker"}}); err != nil {
		t.Fatal(err)
	}
	stored, err := store.LoadSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	turn := stored.Turn
	if !turn.AllowSilence || !turn.NotifyParent || turn.ParentOutput.ReplyTo != "asker" || turn.Output.ReplyTo != "bot" || len(turn.Output.Replies) != 1 || len(stored.QueuedMessages) != 0 {
		t.Fatalf("stored = %+v, turn %+v; want the run's turn, owing the asker too, with its reply", stored, turn)
	}
}
