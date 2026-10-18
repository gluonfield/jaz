package acp_test

import (
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
)

var hourly = loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 * * * *", Timezone: "UTC"}

func (p *jazProcess) newBot(name string) bots.Bot {
	p.t.Helper()
	bot, err := p.bots.Create(p.ctx, bots.CreateBot{Name: name, Agent: acp.AgentCodex})
	if err != nil {
		p.t.Fatal(err)
	}
	return bot
}

// write sends the user's message to thread and waits for its turn to end.
func (p *jazProcess) write(thread, text string) {
	p.t.Helper()
	if _, err := p.manager.Send(p.ctx, acp.SendRequest{Session: thread, Message: text, Completion: acp.CompletionAsync}); err != nil {
		p.t.Fatal(err)
	}
	p.idle(thread)
}

func (p *jazProcess) idle(thread string) {
	p.t.Helper()
	p.eventually("the turn in "+thread+" ending", func() bool {
		session, err := p.store.LoadSession(thread)
		return err == nil && session.Status == storage.StatusIdle && len(session.QueuedMessages) == 0
	})
}

// notices lists what bot posted in its own chat.
func (p *jazProcess) notices(bot string) []string {
	p.t.Helper()
	events, err := p.store.LoadSessionEvents(bot)
	if err != nil {
		p.t.Fatal(err)
	}
	var notices []string
	for _, event := range events {
		if event.RoomMessage != nil && event.RoomMessage.BotID == bot {
			notices = append(notices, event.RoomMessage.Text)
		}
	}
	return notices
}

// guard runs the away guard at a moment of the test's choosing.
func (p *jazProcess) guard(at time.Time) {
	p.t.Helper()
	p.loops.Now = func() time.Time { return at }
	if err := p.loops.GuardAway(); err != nil {
		p.t.Fatal(err)
	}
}

func (p *jazProcess) dueAt(at time.Time) {
	p.t.Helper()
	p.loops.Now = func() time.Time { return at }
	if _, err := p.loops.StartDueAll(p.ctx); err != nil {
		p.t.Fatal(err)
	}
}

func TestThreeDaysAwayHoldBotRoutinesButKeepRunningOnesAcrossARestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	inbox := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Thread: loops.ThreadMain, Schedule: hourly})
	monitor := jaz.routine(loops.CreateLoop{Name: "Monitor", Prompt: "watch the furnace", BotID: bot.ID, Thread: loops.ThreadMain, Schedule: hourly, KeepRunning: true})
	jaz.write(bot.ID, "hello")
	wrote := time.Now()

	jaz.guard(wrote.Add(72*time.Hour - time.Minute))
	if notices := jaz.notices(bot.ID); len(notices) != 0 {
		t.Fatalf("notices = %q a minute before three days, want none", notices)
	}
	slot := wrote.Add(73 * time.Hour).Truncate(time.Hour)
	jaz.guard(slot.Add(time.Second))
	notices := jaz.notices(bot.ID)
	if len(notices) != 1 || !strings.Contains(notices[0], "Inbox") || strings.Contains(notices[0], "Monitor") {
		t.Fatalf("notices = %q, want one naming the routine that waits", notices)
	}
	jaz.dueAt(slot.Add(time.Second))
	if run := jaz.runs(inbox.ID)[0]; run.Status != loops.RunStatusSkipped || !strings.Contains(run.Error, "3 days") {
		t.Fatalf("inbox run = %+v, want it skipped while the user is away", run)
	}
	jaz.waitRun(jaz.runs(monitor.ID)[0].ID, loops.RunStatusOK)

	jaz = jaz.restart(nil)
	jaz.guard(slot.Add(time.Hour + time.Second))
	jaz.dueAt(slot.Add(time.Hour + time.Second))
	if run := jaz.runs(inbox.ID)[0]; run.Status != loops.RunStatusSkipped {
		t.Fatalf("inbox run after a restart = %+v, want it still waiting", run)
	}
	jaz.waitRun(jaz.runs(monitor.ID)[0].ID, loops.RunStatusOK)
	if notices := jaz.notices(bot.ID); len(notices) != 1 {
		t.Fatalf("notices after a restart = %q, want the one already sent", notices)
	}
	if routine, err := jaz.loops.Load(inbox.ID); err != nil || routine.Status != loops.StatusActive {
		t.Fatalf("inbox = %+v, %v; want its own status untouched", routine, err)
	}
}

func TestOnlyTheUsersNextWordsLetRoutinesRunAgain(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	group, err := jaz.bots.CreateGroup("Crew", []string{bot.ID, jaz.newBot("Legolas").ID})
	if err != nil {
		t.Fatal(err)
	}
	inbox := jaz.routine(loops.CreateLoop{Name: "Inbox", Prompt: "check the inbox", BotID: bot.ID, Thread: loops.ThreadMain, Schedule: hourly})
	jaz.routine(loops.CreateLoop{Name: "Digest", Prompt: "write the digest", BotID: bot.ID, Thread: loops.ThreadMain, Schedule: hourly, Status: loops.StatusPaused})
	jaz.write(bot.ID, "hello")
	slot := time.Now().Add(73 * time.Hour).Truncate(time.Hour)
	jaz.guard(slot.Add(time.Second))

	// A hidden turn in the bot's chat and the bot's own group post are not the
	// user.
	if err := jaz.server.QueueInternalTurn(jaz.ctx, bot.ID, storage.NewInternalQueuedMessage("check in quietly")); err != nil {
		t.Fatal(err)
	}
	jaz.idle(bot.ID)
	if err := jaz.bots.Message(bot.ID, group.ID, "status from the bot"); err != nil {
		t.Fatal(err)
	}
	jaz.guard(slot.Add(2 * time.Second))
	if notices := jaz.notices(bot.ID); len(notices) != 1 {
		t.Fatalf("notices = %q, want the routines still waiting", notices)
	}

	if err := jaz.bots.Post(group.ID, "back now", nil); err != nil {
		t.Fatal(err)
	}
	jaz.guard(slot.Add(3 * time.Second))
	notices := jaz.notices(bot.ID)
	if len(notices) != 2 || !strings.Contains(notices[1], "Inbox") || strings.Contains(notices[1], "Digest") {
		t.Fatalf("notices = %q, want the waiting routine back and the paused one left out", notices)
	}
	jaz.dueAt(slot.Add(time.Hour + time.Second))
	jaz.waitRun(jaz.runs(inbox.ID)[0].ID, loops.RunStatusOK)
}
