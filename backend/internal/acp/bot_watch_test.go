package acp_test

import (
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/storage"
)

const owedTurn = "[owed] Your turn ended"

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

func TestABotWhoseThreadEndedShortIsWokenOnceThenTheUserIsToldAcrossARestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	jaz.busy(bot.ID)
	id, err := jaz.commit(bot.ID, bots.MCPCommitmentInput{Request: "Compare the two quotes", Next: "send the comparison"})
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
	jaz.settles(bot.ID, 1, 1)
	view, err := jaz.bots.Activity(bot.ID)
	if err != nil || len(view.Owed) != 1 || !view.Owed[0].Stalled {
		t.Fatalf("owed = %+v, %v; want the commitment stalled", view.Owed, err)
	}
	jaz.write(bot.ID, "Anything else today?")
	jaz.settles(bot.ID, 1, 1)
	if !strings.Contains(jaz.lastPrompt(), "stalled: nothing moved after Jaz reminded you") {
		t.Fatalf("the next turn opened with %q, want the commitment marked stalled", jaz.lastPrompt())
	}

	jaz = jaz.restart(nil)
	jaz.settles(bot.ID, 1, 1)

	// The user takes the thread further; its new result is news the bot owes
	// them, though no turn of the bot's follows.
	jaz.write(thread.ThreadID, "Add the delivery costs.")
	jaz.settles(bot.ID, 2, 2)

	// A wait the bot declares holds until it ends the wait.
	if _, err := jaz.commit(bot.ID, bots.MCPCommitmentInput{ID: id, Waiting: "requester"}); err != nil {
		t.Fatal(err)
	}
	jaz.write(bot.ID, "Which quote do you need from me?")
	jaz.settles(bot.ID, 2, 2)
	jaz.write(bot.ID, "Still deciding.")
	jaz.settles(bot.ID, 2, 2)
	if view, err := jaz.bots.Activity(bot.ID); err != nil || view.Owed[0].Waiting != storage.WaitingRequester || view.Owed[0].Stalled {
		t.Fatalf("owed = %+v, %v; want it waiting on the user", view.Owed, err)
	}
	if _, err := jaz.commit(bot.ID, bots.MCPCommitmentInput{ID: id, Waiting: "none"}); err != nil {
		t.Fatal(err)
	}
	jaz.write(bot.ID, "The second one.")
	jaz.settles(bot.ID, 3, 3)

	if err := jaz.bots.Say(bot.ID, "The second quote is cheaper.", id); err != nil {
		t.Fatal(err)
	}
	jaz.write(bot.ID, "Thanks.")
	jaz.settles(bot.ID, 3, 3)
}

func TestACommitmentThatStalledWhileJazWasDownWakesItsBotAtStartup(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	if _, err := jaz.commit(bot.ID, bots.MCPCommitmentInput{Request: "Book the courier"}); err != nil {
		t.Fatal(err)
	}
	jaz.settles(bot.ID, 0, 0)
	jaz = jaz.restart(nil)
	jaz.settles(bot.ID, 1, 1)
	jaz = jaz.restart(nil)
	jaz.settles(bot.ID, 1, 1)
}
