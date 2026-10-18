package acp_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/integrationingest"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
)

var quotes = &loops.Trigger{Kind: loops.TriggerGmail, Contains: "quote"}

func quote(id, subject string, at time.Time) []integrationingest.Incoming {
	return []integrationingest.Incoming{{Provider: loops.TriggerGmail, ID: id, From: "ujjwal", Subject: subject, At: at}}
}

// children lists the threads routine runs were started in.
func (p *jazProcess) children() []storage.Session {
	p.t.Helper()
	sessions, err := p.store.ListSessions(storage.SessionFilter{SourceType: storage.SourceLoopRun})
	if err != nil {
		p.t.Fatal(err)
	}
	return sessions
}

func TestMessagesWhileARunWaitsFoldIntoIt(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	routine := jaz.routine(loops.CreateLoop{Name: "Quotes", Prompt: "triage the quote", BotID: bot.ID, Trigger: quotes})
	jaz.busy(bot.ID)

	jaz.loops.HandleIncoming(jaz.ctx, quote("m1", "First quote", time.Now()))
	jaz.loops.HandleIncoming(jaz.ctx, quote("m2", "Second quote", time.Now()))
	runs := jaz.runs(routine.ID)
	if len(runs) != 1 || runs[0].Status != loops.RunStatusStarting {
		t.Fatalf("runs = %+v, want one pending run", runs)
	}
	jaz.release()
	jaz.waitRun(runs[0].ID, loops.RunStatusOK)
	prompts := jaz.prompts()
	if last := prompts[len(prompts)-1]; len(prompts) != 2 || !strings.Contains(last, "First quote") || !strings.Contains(last, "Second quote") {
		t.Fatalf("prompts = %q, want one routine turn with both messages", prompts)
	}
}

func TestMessagesDuringABegunChildRunWaitForOneFollowUp(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	routine := jaz.routine(loops.CreateLoop{Name: "Quotes", Prompt: "triage the quote until released", Trigger: quotes})

	jaz.loops.HandleIncoming(jaz.ctx, quote("m1", "First quote", time.Now()))
	first := jaz.runs(routine.ID)[0]
	waitForBlockingTool(t, jaz.ctx, jaz.manager, jaz.waitRun(first.ID, loops.RunStatusRunning).ThreadID)
	jaz.loops.HandleIncoming(jaz.ctx, quote("m2", "Second quote", time.Now()))
	jaz.loops.HandleIncoming(jaz.ctx, quote("m3", "Third quote", time.Now()))
	runs := jaz.runs(routine.ID)
	if len(runs) != 2 || runs[0].Status != loops.RunStatusStarting || len(jaz.children()) != 1 {
		t.Fatalf("runs = %+v, children = %d; want one follow-up waiting, not running beside the first", runs, len(jaz.children()))
	}

	jaz.release()
	jaz.waitRun(first.ID, loops.RunStatusOK)
	follow := jaz.waitRun(runs[0].ID, loops.RunStatusOK)
	if finished := jaz.run(first.ID).FinishedAt; follow.StartedAt.Before(finished) {
		t.Fatalf("follow-up started %s, before the first run finished %s", follow.StartedAt, finished)
	}
	prompts := jaz.prompts()
	if last := prompts[len(prompts)-1]; !strings.Contains(last, "Second quote") || !strings.Contains(last, "Third quote") || strings.Contains(last, "First quote") {
		t.Fatalf("follow-up prompt = %q, want the two later messages", last)
	}
}

func TestATakenMessageStaysTakenAfterARestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	routine := jaz.routine(loops.CreateLoop{Name: "Quotes", Prompt: "triage the quote", BotID: bot.ID, Trigger: quotes})
	at := time.Now()
	jaz.loops.HandleIncoming(jaz.ctx, quote("m1", "First quote", at))
	jaz.waitRun(jaz.runs(routine.ID)[0].ID, loops.RunStatusOK)

	jaz = jaz.restart(nil)
	jaz.loops.HandleIncoming(jaz.ctx, quote("m1", "First quote", at))
	if runs := jaz.runs(routine.ID); len(runs) != 1 {
		t.Fatalf("runs = %+v, want the resynced message to stay taken", runs)
	}
}

func TestACatchUpRoutineTakesMessagesMissedWhileJazWasDown(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	down := time.Now().Add(-3 * time.Hour)
	jaz.loops.Now = func() time.Time { return down.Add(-time.Hour) }
	routines := map[string]loops.Loop{}
	for _, whenMissed := range []string{loops.MissedSkip, loops.MissedCatchUp} {
		routines[whenMissed] = jaz.routine(loops.CreateLoop{Name: whenMissed, Prompt: "triage the quote", BotID: bot.ID, Trigger: quotes, WhenMissed: whenMissed})
	}
	jaz.loops.Now = func() time.Time { return down }
	jaz.loops.HandleIncoming(jaz.ctx, quote("m1", "First quote", down))
	for _, routine := range routines {
		jaz.waitRun(jaz.runs(routine.ID)[0].ID, loops.RunStatusOK)
	}

	// Back three hours later, the mail sync brings what arrived meanwhile and
	// older history.
	jaz = jaz.restart(nil)
	jaz.loops.HandleIncoming(jaz.ctx, append(quote("m2", "Quote sent while down", down.Add(time.Hour)), quote("m0", "Quote from before", down.Add(-30*time.Minute))...))
	if runs := jaz.runs(routines[loops.MissedSkip].ID); len(runs) != 1 {
		t.Fatalf("the skipping routine ran again: %+v", runs)
	}
	runs := jaz.runs(routines[loops.MissedCatchUp].ID)
	if len(runs) != 2 || !strings.Contains(runs[0].Reason, "Quote sent while down") || strings.Contains(runs[0].Reason, "Quote from before") {
		t.Fatalf("catch-up runs = %+v, want one run with the message missed while down", runs)
	}
	jaz.waitRun(runs[0].ID, loops.RunStatusOK)
}

func TestAFollowUpWaitsBehindItsResumedRunAfterARestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	routine := jaz.routine(loops.CreateLoop{Name: "Quotes", Prompt: "triage the quote until released", Trigger: quotes})
	jaz.loops.HandleIncoming(jaz.ctx, quote("m1", "First quote", time.Now()))
	first := jaz.waitRun(jaz.runs(routine.ID)[0].ID, loops.RunStatusRunning)
	waitForBlockingTool(t, jaz.ctx, jaz.manager, first.ThreadID)
	jaz.loops.HandleIncoming(jaz.ctx, quote("m2", "Second quote", time.Now()))
	follow := jaz.runs(routine.ID)[0]

	jaz = jaz.restart(map[string]string{"JAZ_FAKE_ACP_HOLD_TEXT": "Continue from where you left off"})
	waitForBlockingTool(t, jaz.ctx, jaz.manager, first.ThreadID)
	// Long enough for a follow-up started too early to have begun.
	time.Sleep(200 * time.Millisecond)
	if run := jaz.run(follow.ID); run.Status != loops.RunStatusStarting || len(jaz.children()) != 1 {
		t.Fatalf("follow-up = %+v with %d children, want it waiting behind the resumed run", run, len(jaz.children()))
	}
	jaz.release()
	finished := jaz.waitRun(first.ID, loops.RunStatusOK).FinishedAt
	if started := jaz.waitRun(follow.ID, loops.RunStatusOK).StartedAt; started.Before(finished) {
		t.Fatalf("follow-up started %s, before its resumed run finished %s", started, finished)
	}
}

func TestRestartAcrossAMissedSlotCatchesUpOnceAndSkipsTheJoke(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.bot("Gimli")
	slot := time.Date(2026, 6, 8, 9, 0, 0, 0, time.UTC)
	jaz.loops.Now = func() time.Time { return slot.Add(-10 * time.Minute) }
	daily := loops.Schedule{Kind: loops.ScheduleCron, Expr: "0 9 * * *", Timezone: "UTC"}
	sync := jaz.routine(loops.CreateLoop{Name: "Sync", Prompt: "sync the notes", BotID: bot.ID, WhenMissed: loops.MissedCatchUp, Schedule: daily})
	joke := jaz.routine(loops.CreateLoop{Name: "Joke", Prompt: "tell a joke", BotID: bot.ID, Schedule: daily})

	// Jaz stops before 09:00 and starts again at 09:40.
	jaz = jaz.restart(nil)
	jaz.loops.Now = func() time.Time { return slot.Add(40 * time.Minute) }
	ctx, stop := context.WithCancel(jaz.ctx)
	defer stop()
	go func() { _ = loops.StartScheduler(ctx, jaz.loops, 10*time.Millisecond) }()
	jaz.eventually("both routines settled", func() bool { return len(jaz.runs(sync.ID)) == 1 && len(jaz.runs(joke.ID)) == 1 })
	stop()
	caughtUp := jaz.runs(sync.ID)[0]
	if caughtUp.Reason != "Catching up: Jaz was not running at Mon 8 Jun 09:00 UTC. No run of this routine has finished successfully yet." {
		t.Fatalf("sync run = %+v", caughtUp)
	}
	jaz.waitRun(caughtUp.ID, loops.RunStatusOK)
	if skipped := jaz.runs(joke.ID)[0]; skipped.Status != loops.RunStatusSkipped {
		t.Fatalf("joke run = %+v, want it skipped", skipped)
	}
}
