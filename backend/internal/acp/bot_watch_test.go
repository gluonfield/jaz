package acp_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/storage"
)

const owedTurn = "[owed] Your turn ended"

func TestBotTaskWatchRecoversWithoutATurnEventAndStaysQuietWhileWorkersRun(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	id := owe(t, store, bot.ID)
	ctx, cancel := context.WithCancel(jaz.ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		jaz.bots.RunWatch(ctx, 5*time.Millisecond)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	jaz.settles(bot.ID, 1, 0)
	cancel()
	<-done
	if _, err := jaz.commit(bot.ID, bots.Commit{ID: id, Status: storage.CommitmentDone, Message: "The work is delivered."}); err != nil {
		t.Fatal(err)
	}
	id = owe(t, store, bot.ID)
	_, worker, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{
		Prompt: "Work until released", Commitment: id,
	})
	if err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, worker.ThreadID)
	for range 20 {
		if err := jaz.bots.WatchAll(); err != nil {
			t.Fatal(err)
		}
	}
	if got := jaz.promptsContaining(owedTurn); got != 1 {
		t.Fatalf("healthy workers caused %d recovery wakes; want only the original orphan wake", got)
	}
}

// stalledNotices lists what bot's chat told the user had stalled.
func (p *jazProcess) stalledNotices(bot string) int {
	count := 0
	for _, notice := range p.notices(bot) {
		if strings.HasPrefix(notice, "Stalled: ") {
			count++
		}
	}
	return count
}

// settles waits until bot's chat has ended its turn with nothing queued and no
// turn end left to account for, which nothing but a new event changes, and
// checks it was woken wakes times and the user told told times.
func (p *jazProcess) settles(bot string, wakes, told int) {
	p.t.Helper()
	p.eventually("the bot settling after its reminders", func() bool {
		if got := p.promptsContaining(owedTurn); got > wakes {
			p.t.Fatalf("the bot was woken %d times, want %d", got, wakes)
		}
		if got := p.stalledNotices(bot); got > told {
			p.t.Fatalf("the user was told %d times that it stalled, want %d", got, told)
		}
		session, err := p.store.LoadSession(bot)
		return err == nil && session.Status == storage.StatusIdle && len(session.QueuedMessages) == 0 && p.finishing.Load() == 0 &&
			p.promptsContaining(owedTurn) == wakes && p.stalledNotices(bot) == told
	})
}

func TestABotRecoversUnfinishedWorkAndReleasesAQuestionWaitAcrossARestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	jaz.busy(bot.ID)
	id, err := jaz.commit(bot.ID, bots.Commit{Request: "Compare the two quotes", Next: "send the comparison"})
	if err != nil {
		t.Fatal(err)
	}
	tools := acp.NewMCPTools(jaz.manager)
	_, thread, err := tools.Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: "quotes", Prompt: "Compare the two quotes until released", Commitment: id})
	if err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, thread.ThreadID)
	jaz.cancel(bot.ID)
	jaz.settles(bot.ID, 0, 0)
	jaz.write(bot.ID, "How is it going?")
	jaz.settles(bot.ID, 0, 0)

	// The thread's result reaches the bot, whose turn ends without delivering.
	jaz.release()
	jaz.settles(bot.ID, 1, 0)
	view, err := jaz.bots.Activity(bot.ID)
	if err != nil || len(view.Owed) != 1 || !view.Owed[0].Stalled {
		t.Fatalf("owed = %+v, %v; want the commitment stalled", view.Owed, err)
	}
	jaz.write(bot.ID, "Anything else today?")
	jaz.settles(bot.ID, 1, 0)

	jaz = jaz.restart(nil)
	jaz.settles(bot.ID, 1, 0)

	// The user takes the thread further; its new result is news the bot owes
	// them, though no turn of the bot's follows.
	jaz.write(thread.ThreadID, "Add the delivery costs.")
	jaz.settles(bot.ID, 2, 0)

	if _, err := jaz.commit(bot.ID, bots.Commit{ID: id, Status: storage.CommitmentWaitingForUser, Message: "Which quote should I use?"}); err != nil {
		t.Fatal(err)
	}
	if view, err := jaz.bots.Activity(bot.ID); err != nil || view.Owed[0].Status != storage.CommitmentWaitingForUser || view.Owed[0].QuestionSeq == 0 || view.Owed[0].Stalled {
		t.Fatalf("owed = %+v, %v; want a real unanswered question", view.Owed, err)
	}
	jaz.write(bot.ID, "The second one.")
	jaz.settles(bot.ID, 3, 0)

	if err := jaz.bots.Say(bot.ID, "The second quote is cheaper.", id); err != nil {
		t.Fatal(err)
	}
	jaz.write(bot.ID, "Thanks.")
	jaz.settles(bot.ID, 3, 0)
}

func TestACommitmentThatStalledWhileJazWasDownWakesItsBotAtStartup(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	if _, err := jaz.commit(bot.ID, bots.Commit{Request: "Book the courier"}); err != nil {
		t.Fatal(err)
	}
	jaz.settles(bot.ID, 0, 0)
	jaz = jaz.restart(nil)
	jaz.settles(bot.ID, 1, 0)
	jaz = jaz.restart(nil)
	jaz.settles(bot.ID, 1, 0)
}

func TestABotIsNotWokenWhileItsThreadWorksInTheBackground(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	jaz.busy(bot.ID)
	id, err := jaz.commit(bot.ID, bots.Commit{Request: "Run the login suites"})
	if err != nil {
		t.Fatal(err)
	}
	// The thread's turn ends while the suites it started still run.
	_, thread, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: "suites", Prompt: "Run the login suites: task suites=running", Commitment: id})
	if err != nil {
		t.Fatal(err)
	}
	jaz.release()
	jaz.idle(thread.ThreadID)
	jaz.settles(bot.ID, 0, 0)
	if commitment := jaz.commitment(id); !commitment.Moving || commitment.Threads[0].State != "background" {
		t.Fatalf("commitment = %+v, want its thread working in the background", commitment)
	}

	// The suites finish in a turn of the thread's.
	jaz.write(thread.ThreadID, "The suites finished: task suites=completed")
	jaz.settles(bot.ID, 1, 0)

	// Background work does not outlive its agent, so the stall it hid is new.
	jaz.write(thread.ThreadID, "Run them once more: task again=running")
	jaz.settles(bot.ID, 1, 0)
	jaz.manager.Close()
	if commitment := jaz.commitment(id); commitment.Moving {
		t.Fatalf("commitment = %+v, want a stopped agent's background work gone", commitment)
	}
	// A server that dies with its agents leaves the mark for the next one.
	if err := store.SetBackgroundWork(thread.ThreadID, true); err != nil {
		t.Fatal(err)
	}
	jaz = jaz.restart(nil)
	jaz.settles(bot.ID, 2, 0)
}
