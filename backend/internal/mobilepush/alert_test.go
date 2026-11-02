package mobilepush

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/sessionevents"
	"github.com/wins/jaz/backend/internal/storage"
)

// told is what a listening desktop hears while service processes events.
func told(t *testing.T, service *Service, events ...sessionevents.Event) []Alert {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	alerts := service.Subscribe(ctx)
	for _, event := range events {
		if err := service.process(ctx, event); err != nil {
			t.Fatal(err)
		}
	}
	var heard []Alert
	for {
		select {
		case alert := <-alerts:
			heard = append(heard, alert)
		default:
			return heard
		}
	}
}

func ended(id string, finished sessionevents.ACPEvent) sessionevents.Event {
	return sessionevents.Event{SessionID: id, Type: sessionevents.TypeTurnFinished, At: time.Now(), ACP: &finished}
}

func TestPlainThreadTellsItsReplyFailuresAndQuestionsFromBelow(t *testing.T) {
	service, store, _, session := pushFixture(t, "")
	child, err := store.CreateSession(storage.CreateSession{Title: "Helper", ParentID: session.ID, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{session.ID, child.ID} {
		if err := store.CompleteSession(id, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	quiet := told(t, service,
		ended(session.ID, sessionevents.ACPEvent{State: "cancelled", StopReason: "cancelled"}),
		ended(session.ID, sessionevents.ACPEvent{State: "interrupted"}),
		ended(session.ID, sessionevents.ACPEvent{State: "idle", StopReason: "end_turn"}),
		ended(child.ID, sessionevents.ACPEvent{State: "idle", StopReason: "end_turn", Assistant: "Done."}),
	)
	if len(quiet) != 0 {
		t.Fatalf("a cancel, restart, compaction or child turn told the person: %#v", quiet)
	}
	heard := told(t, service,
		ended(session.ID, sessionevents.ACPEvent{State: "idle", StopReason: "end_turn", Assistant: "## Summary\n**Fixed** the [build](https://example.com). Tests pass too."}),
		ended(session.ID, sessionevents.ACPEvent{State: "failed", Error: "provider overloaded\n  at retry.go:12"}),
		ended(session.ID, sessionevents.ACPEvent{State: "failed"}),
		sessionevents.Event{SessionID: child.ID, Type: "permission_request", Permission: &sessionevents.ACPPermission{
			ID: "question", SessionID: child.ID, Questions: []sessionevents.ACPQuestion{{ID: "q", Question: "Which supplier?"}},
		}},
	)
	want := []Alert{
		{SessionID: session.ID, State: "reply", Body: "Fixed the build."},
		{SessionID: session.ID, State: "stopped", Body: "provider overloaded"},
		{SessionID: session.ID, State: "stopped", Body: "Work stopped with an error."},
		{SessionID: child.ID, State: "needs_you", Body: "Which supplier?", RequestID: "question"},
	}
	if len(heard) != len(want) {
		t.Fatalf("heard %#v", heard)
	}
	for i, alert := range heard {
		if alert.SessionID != want[i].SessionID || alert.State != want[i].State || alert.Body != want[i].Body || alert.RequestID != want[i].RequestID || alert.Kind != "chat" {
			t.Fatalf("alert %d = %#v; want %#v", i, alert, want[i])
		}
	}
	if heard[0].Title != "Research" {
		t.Fatalf("a thread's reply did not carry its title: %#v", heard[0])
	}
}

// published collects what a bot service streams.
type published []sessionevents.Event

func (p *published) Publish(event sessionevents.Event) {
	*p = append(*p, event)
}

func TestBotTellsTaskOutcomesAndNewRepliesButNotAcknowledgements(t *testing.T) {
	service, store, _, bot := pushFixture(t, storage.SourceBot)
	var sent published
	botService := bots.NewService(store, t.TempDir(), nil, nil, nil, nil, &sent, log.New(io.Discard))
	turn := func(user bool) time.Time {
		t.Helper()
		time.Sleep(time.Millisecond)
		if err := store.FinishSessionTurn(bot.ID, storage.StatusIdle, "", time.Now(), nil); err != nil {
			t.Fatal(err)
		}
		if claimed, err := store.ClaimTurn(bot.ID, storage.Turn{Output: &storage.TurnOutput{User: user}}); err != nil || !claimed {
			t.Fatalf("claim turn: %v %v", claimed, err)
		}
		session, err := store.LoadSession(bot.ID)
		if err != nil {
			t.Fatal(err)
		}
		return session.Turn.StartedAt
	}
	commit := func(change bots.Commit) int64 {
		t.Helper()
		id, err := botService.Commit(bot.ID, change)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	say := func(text string) {
		t.Helper()
		if err := botService.Say(bot.ID, text); err != nil {
			t.Fatal(err)
		}
	}
	finished := func(started time.Time) sessionevents.Event {
		return ended(bot.ID, sessionevents.ACPEvent{State: "idle", StopReason: "end_turn", Assistant: "Private narration.", TurnStartedAt: started})
	}
	hear := func(more ...sessionevents.Event) []Alert {
		heard := told(t, service, append(sent, more...)...)
		sent = nil
		return heard
	}
	asked := turn(true)
	price := commit(bots.Commit{Request: "Price the part"})
	say("On it.")
	quiet := hear(finished(asked))
	working := turn(false)
	say("Still checking.")
	quiet = append(quiet, hear(finished(working))...)
	if len(quiet) != 0 {
		t.Fatalf("acknowledgement or progress told the person: %#v", quiet)
	}
	answered := turn(true)
	commit(bots.Commit{ID: price, Status: storage.CommitmentDone, Message: "Four pounds. The cheapest of three."})
	heard := hear(finished(answered))
	chatted := turn(true)
	say("It is sunny.")
	heard = append(heard, hear(finished(chatted))...)
	heard = append(heard, hear(finished(turn(true)))...)
	if len(heard) != 2 || heard[0].State != "done" || heard[0].Title != "Price the part" || heard[0].Body != "Four pounds." {
		t.Fatalf("a delivered task was not told once with its outcome: %#v", heard)
	}
	if heard[1].State != "reply" || heard[1].Title != "" || heard[1].Body != "It is sunny." {
		t.Fatalf("a new reply was not told: %#v", heard[1])
	}
	if heard[0].Kind != "bot" || heard[0].Name != "Research" || len(heard[0].Avatars) != 1 || heard[0].Avatars[0] != (bots.Avatar{Shape: "circle", Color: "blue"}) {
		t.Fatalf("a bot alert did not carry who it is: %#v", heard[0])
	}
	turn(false)
	order := commit(bots.Commit{Request: "Order the part"})
	commit(bots.Commit{ID: order, Status: storage.CommitmentWaitingForUser, Message: "Which size? The small one is cheaper."})
	stock := commit(bots.Commit{Request: "Check the stock"})
	commit(bots.Commit{ID: stock, Status: storage.CommitmentStopped, Message: "Supplier closed: no stock until May. Retrying will not help.\nTried twice."})
	commit(bots.Commit{ID: stock, Status: storage.CommitmentPending, Message: "The supplier restocked early."})
	heard = hear()
	if len(heard) != 2 || heard[0].State != "needs_you" || heard[0].Title != "Order the part" || heard[0].Body != "Which size?" {
		t.Fatalf("a task question was not told: %#v", heard)
	}
	if heard[1].State != "stopped" || heard[1].Title != "Check the stock" || heard[1].Body != "Supplier closed: no stock until May. Retrying will not help." {
		t.Fatalf("a stopped task was not told with its reason: %#v", heard[1])
	}
}

func TestGroupTellsAMembersReplyAsTheGroup(t *testing.T) {
	service, store, _, member := pushFixture(t, storage.SourceBot)
	group, err := store.CreateSession(storage.CreateSession{Title: "Team", SourceType: storage.SourceBot, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBot(storage.BotRecord{ThreadID: group.ID, Kind: bots.KindGroup, Members: []string{member.ID}}); err != nil {
		t.Fatal(err)
	}
	thread, err := store.CreateSession(storage.CreateSession{Title: "Research in Team", SourceType: storage.SourceBotMember, SourceID: member.ID, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMembership(storage.BotMembership{GroupID: group.ID, BotID: member.ID, ThreadID: thread.ID}); err != nil {
		t.Fatal(err)
	}
	var sent published
	botService := bots.NewService(store, t.TempDir(), nil, nil, nil, nil, &sent, log.New(io.Discard))
	// Another member's post started the turn; the person's post was then
	// handed to it, and a later post landed before the turn ended.
	if claimed, err := store.ClaimTurn(thread.ID, storage.Turn{}); err != nil || !claimed {
		t.Fatalf("claim turn: %v %v", claimed, err)
	}
	if err := store.UpdateTurn(thread.ID, storage.TurnUpdate{User: true}); err != nil {
		t.Fatal(err)
	}
	if err := botService.Say(thread.ID, "Shipped it. Notes follow."); err != nil {
		t.Fatal(err)
	}
	later, err := store.DeliverMessage(sessionevents.Event{
		SessionID: group.ID, Type: sessionevents.TypeRoomMessage,
		RoomMessage: &sessionevents.RoomMessageEvent{Speaker: "bot", BotID: "other", Text: "Nice."},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	running, err := store.LoadSession(thread.ID)
	if err != nil {
		t.Fatal(err)
	}
	heard := told(t, service, append(sent, later, ended(thread.ID, sessionevents.ACPEvent{State: "idle", StopReason: "end_turn", TurnStartedAt: running.Turn.StartedAt}))...)
	if len(heard) != 1 || heard[0].SessionID != group.ID || heard[0].Kind != bots.KindGroup || heard[0].Name != "Team" || heard[0].State != "reply" || heard[0].Body != "Shipped it." || len(heard[0].Avatars) != 1 {
		t.Fatalf("a member's reply was not told as its group's: %#v", heard)
	}
}

func TestQuestionsFromThreadsWorkingForABotAskAsItWhereTheyAreAnswered(t *testing.T) {
	service, store, _, bot := pushFixture(t, storage.SourceBot)
	create := func(title, source, parent string) storage.Session {
		t.Helper()
		session, err := store.CreateSession(storage.CreateSession{Title: title, SourceType: source, SourceID: bot.ID, ParentID: parent, Runtime: storage.RuntimeACP})
		if err != nil {
			t.Fatal(err)
		}
		return session
	}
	worker := create("Price the part", storage.SourceBotWorker, bot.ID)
	below := create("Check the drawing", storage.SourceBotWorker, worker.ID)
	member := create("Research in Team", storage.SourceBotMember, "")
	group, err := store.CreateSession(storage.CreateSession{Title: "Team", SourceType: storage.SourceBot, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBot(storage.BotRecord{ThreadID: group.ID, Kind: bots.KindGroup, Members: []string{bot.ID}}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveMembership(storage.BotMembership{GroupID: group.ID, BotID: bot.ID, ThreadID: member.ID}); err != nil {
		t.Fatal(err)
	}
	background, err := store.CreateSession(storage.CreateSession{Title: "Dream", SourceType: storage.SourceMemoryDream, Runtime: storage.RuntimeACP})
	if err != nil {
		t.Fatal(err)
	}
	ask := func(at, asker string) sessionevents.Event {
		return sessionevents.Event{SessionID: at, Type: "permission_request", Permission: &sessionevents.ACPPermission{
			ID: "q-" + asker, SessionID: asker, Questions: []sessionevents.ACPQuestion{{ID: "q", Question: "Which supplier?"}},
		}}
	}
	heard := told(t, service,
		ask(worker.ID, worker.ID), ask(bot.ID, worker.ID),
		ask(below.ID, below.ID), ask(worker.ID, below.ID),
		ask(member.ID, member.ID),
		ask(background.ID, background.ID),
	)
	want := []storage.Session{worker, below, member}
	if len(heard) != len(want) {
		t.Fatalf("heard %#v", heard)
	}
	for i, alert := range heard {
		if alert.SessionID != want[i].ID || alert.Kind != "chat" || alert.Title != want[i].Title || alert.State != "needs_you" || alert.RequestID != "q-"+want[i].ID || alert.Body != "Which supplier?" {
			t.Fatalf("question %d did not open where it is answered: %#v", i, alert)
		}
		if alert.Name != "Research" || len(alert.Avatars) != 1 || alert.Avatars[0] != (bots.Avatar{Shape: "circle", Color: "blue"}) {
			t.Fatalf("question %d did not ask as its bot: %#v", i, alert)
		}
	}
}
