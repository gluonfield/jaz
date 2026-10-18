package sqlite

import (
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

func TestTurnCompletionAndDeliveryAreAtomicAndSurviveReopen(t *testing.T) {
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
	missing := storage.TurnDelivery{SessionID: "missing", Message: storage.NewInternalQueuedMessage("other result")}
	if err := store.FinishSessionTurn(child.ID, storage.StatusIdle, "", time.Now(), []storage.TurnDelivery{delivery, missing}); err == nil {
		t.Fatal("completion committed despite unavailable delivery target")
	}
	storedChild, _ := store.LoadSession(child.ID)
	storedParent, _ := store.LoadSession(parent.ID)
	if storedChild.Status != storage.StatusRunning || storedChild.Turn == nil || len(storedParent.QueuedMessages) != 0 {
		t.Fatalf("partial completion: child=%+v parent=%+v", storedChild, storedParent)
	}
	for range 2 {
		if err := store.FinishSessionTurn(child.ID, storage.StatusIdle, "", time.Now(), []storage.TurnDelivery{delivery}); err != nil {
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
	storedChild, _ = store.LoadSession(child.ID)
	storedParent, _ = store.LoadSession(parent.ID)
	if storedChild.Status != storage.StatusIdle || storedChild.Turn != nil || len(storedParent.QueuedMessages) != 1 || storedParent.QueuedMessages[0].Text != "result" {
		t.Fatalf("reopened completion: child=%+v parent=%+v", storedChild, storedParent)
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
	if err := store.AppendTurnReply(session.ID, "Reply after intent was read"); err != nil {
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
