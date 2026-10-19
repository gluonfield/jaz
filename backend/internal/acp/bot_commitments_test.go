package acp_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/wins/jaz/backend/internal/acp"
	"github.com/wins/jaz/backend/internal/bots"
	"github.com/wins/jaz/backend/internal/loops"
	"github.com/wins/jaz/backend/internal/storage"
	sqlitestore "github.com/wins/jaz/backend/internal/storage/sqlite"
)

// owe opens a commitment of bot for the threads a test starts from it.
func owe(t *testing.T, store *sqlitestore.Store, bot string) int64 {
	t.Helper()
	commitment, err := store.OpenCommitment(storage.Commitment{BotID: bot, Request: "the test's work"})
	if err != nil {
		t.Fatal(err)
	}
	return commitment.ID
}

// commit opens or changes one of bot's commitments the way its agent's
// commitment tool does.
func (p *jazProcess) commit(bot string, input bots.MCPCommitmentInput) (int64, error) {
	_, out, err := bots.NewMCPTools(p.bots).Commit(p.ctx, callAs(bot), input)
	return out.ID, err
}

func (p *jazProcess) commitment(id int64) storage.Commitment {
	p.t.Helper()
	commitment, err := p.store.LoadCommitment(id)
	if err != nil {
		p.t.Fatal(err)
	}
	return commitment
}

// lastPrompt is the last prompt bot's agent was sent.
func (p *jazProcess) lastPrompt() string {
	p.t.Helper()
	prompts := p.prompts()
	if len(prompts) == 0 {
		p.t.Fatal("no prompts")
	}
	return prompts[len(prompts)-1]
}

func TestABotsCommitmentLinksItsThreadAndClosesOnlyWhenTheResultReachesTheUserAcrossARestart(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	jaz.busy(bot.ID)
	tools := acp.NewMCPTools(jaz.manager)
	if _, _, err := tools.Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Prompt: "Compare the two quotes until released"}); err == nil || !strings.Contains(err.Error(), "commitment") {
		t.Fatalf("a thread started without a commitment: %v", err)
	}
	id, err := jaz.commit(bot.ID, bots.MCPCommitmentInput{Request: "Compare the two quotes", Next: "send the comparison"})
	if err != nil {
		t.Fatal(err)
	}
	_, thread, err := tools.Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Slug: "quotes", Prompt: "Compare the two quotes until released", Commitment: id})
	if err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, thread.ThreadID)
	if commitment := jaz.commitment(id); commitment.Requester != "" || len(commitment.Threads) != 1 || commitment.Threads[0].ID != thread.ThreadID {
		t.Fatalf("commitment = %+v, want it owed to the user and serving the thread", commitment)
	}
	if err := jaz.bots.Say(bot.ID, "Here it is.", id); err == nil || !strings.Contains(err.Error(), "still working") {
		t.Fatalf("closed while its thread works: %v", err)
	}

	jaz = jaz.restart(nil)
	jaz.release()
	jaz.eventually("the thread's result reaching the bot", func() bool {
		return strings.Contains(jaz.lastPrompt(), "ACP session quotes")
	})
	if note := fmt.Sprintf("#%d for the user: Compare the two quotes · thread quotes finished · next: send the comparison", id); !strings.Contains(jaz.lastPrompt(), note) {
		t.Fatalf("the result turn opened with %q, want the note %q", jaz.lastPrompt(), note)
	}
	if commitment := jaz.commitment(id); commitment.Status != storage.CommitmentOpen {
		t.Fatalf("closed before delivery: %+v", commitment)
	}
	if err := jaz.bots.Say(bot.ID, "The first quote is cheaper.", id); err != nil {
		t.Fatal(err)
	}
	if commitment := jaz.commitment(id); commitment.Status != storage.CommitmentDone || commitment.ClosedAt.IsZero() {
		t.Fatalf("delivered commitment = %+v, want done", commitment)
	}
	jaz.idle(bot.ID)
	session, err := store.LoadSession(bot.ID)
	if err != nil {
		t.Fatal(err)
	}
	if note, _ := bots.TurnNote(store)(session); strings.Contains(note, fmt.Sprintf("#%d ", id)) {
		t.Fatalf("a closed commitment stayed in the bot's note: %q", note)
	}
}

func TestACommitmentToABotClosesWithTheAnswerThatReachesIt(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	asker, helper := jaz.newBot("Asker"), jaz.newBot("Helper")
	opened := make(chan int64, 1)
	jaz.server.ACP = earlyReplyManager{Manager: jaz.manager, speak: func(req acp.InternalTurnRequest) error {
		if req.Session != helper.ID {
			return nil
		}
		if strings.HasPrefix(req.Message, "[message from Asker]") {
			// Answered later, without a thread: a check-back.
			id, err := jaz.commit(helper.ID, bots.MCPCommitmentInput{Request: "Tell Asker the price", Next: "ask the user"})
			opened <- id
			return err
		}
		return nil
	}}
	if err := jaz.bots.Message(asker.ID, helper.ID, "What does the part cost?"); err != nil {
		t.Fatal(err)
	}
	id := <-opened
	jaz.idle(helper.ID)
	if err := jaz.bots.Say(helper.ID, "It costs four pounds.", id); err == nil || !strings.Contains(err.Error(), "owed to Asker") {
		t.Fatalf("a commitment to Asker closed with a message to the user: %v", err)
	}
	if err := jaz.bots.Message(helper.ID, asker.ID, "It costs four pounds.", id); err != nil {
		t.Fatal(err)
	}
	if commitment := jaz.commitment(id); commitment.Status != storage.CommitmentDone {
		t.Fatalf("commitment = %+v, want done once Asker has the answer", commitment)
	}
	jaz.eventually("the answer reaching Asker", func() bool {
		return jaz.promptsContaining("It costs four pounds.") == 1
	})
}

func TestADroppedCommitmentClosesWithItsReasonOnceItsThreadStopped(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	bot := jaz.newBot("Gimli")
	id, err := jaz.commit(bot.ID, bots.MCPCommitmentInput{Request: "Draft the brochure"})
	if err != nil {
		t.Fatal(err)
	}
	_, thread, err := acp.NewMCPTools(jaz.manager).Create(jaz.ctx, callAs(bot.ID), acp.MCPCreateInput{Prompt: "Draft the brochure until released", Commitment: id})
	if err != nil {
		t.Fatal(err)
	}
	waitForBlockingTool(t, jaz.ctx, jaz.manager, thread.ThreadID)
	if _, err := jaz.commit(bot.ID, bots.MCPCommitmentInput{ID: id, Drop: "the user cancelled it"}); err == nil {
		t.Fatal("dropped while its thread works")
	}
	jaz.cancel(thread.ThreadID)
	jaz.eventually("the thread stopping", func() bool {
		session, err := store.LoadSession(thread.ThreadID)
		return err == nil && session.Status != storage.StatusRunning
	})
	if _, err := jaz.commit(bot.ID, bots.MCPCommitmentInput{ID: id, Drop: "the user cancelled it"}); err != nil {
		t.Fatal(err)
	}
	if commitment := jaz.commitment(id); commitment.Status != storage.CommitmentDropped || commitment.Note != "the user cancelled it" {
		t.Fatalf("commitment = %+v, want dropped with its reason", commitment)
	}
	if _, err := jaz.commit(bot.ID, bots.MCPCommitmentInput{ID: id, Next: "start again"}); err == nil {
		t.Fatal("a dropped commitment took a new step")
	}
}

func TestAnAnswerWithDoneGoesToTheThreadThatAsked(t *testing.T) {
	store, root := newTestStore(t)
	jaz := startJaz(t, testContext(t), store, root, map[string]string{})
	asker, helper := jaz.newBot("Asker"), jaz.newBot("Helper")
	routine := jaz.routine(loops.CreateLoop{Name: "Scout", Prompt: "scout until released", BotID: asker.ID})
	run, err := jaz.loops.RunNow(jaz.ctx, routine.ID)
	if err != nil {
		t.Fatal(err)
	}
	child := jaz.waitRun(run.ID, loops.RunStatusRunning).ThreadID
	opened := make(chan int64, 1)
	replies := make(chan string, 1)
	jaz.server.ACP = earlyReplyManager{Manager: jaz.manager, speak: func(req acp.InternalTurnRequest) error {
		switch {
		case req.Session == helper.ID && strings.HasPrefix(req.Message, "[message from Asker]"):
			id, err := jaz.commit(helper.ID, bots.MCPCommitmentInput{Request: "Price the part"})
			opened <- id
			return err
		case req.Session == child && strings.HasPrefix(req.Message, "[message from Helper]"):
			return jaz.bots.Say(child, "Thanks, noted.")
		case req.Session == helper.ID:
			replies <- req.Message
		}
		return nil
	}}
	// Asker's routine asks from a thread of its own.
	if err := jaz.bots.Message(child, "Helper", "What does the part cost?"); err != nil {
		t.Fatal(err)
	}
	id := <-opened
	jaz.idle(helper.ID)
	if err := jaz.bots.Say(helper.ID, "It costs four pounds.", id); err == nil || !strings.Contains(err.Error(), "owed to Asker") {
		t.Fatalf("an answer to the user closed what Asker asked: %v", err)
	}
	if err := jaz.bots.Message(helper.ID, "Asker", "It costs four pounds.", id); err != nil {
		t.Fatal(err)
	}
	if commitment := jaz.commitment(id); commitment.Requester != child || commitment.Status != storage.CommitmentDone {
		t.Fatalf("commitment = %+v, want it closed by the answer to the routine's thread", commitment)
	}
	answer := func(thread string) bool {
		for _, message := range jaz.queued(thread) {
			if strings.Contains(message.Text, "It costs four pounds.") {
				return true
			}
		}
		return false
	}
	if !answer(child) || answer(asker.ID) {
		t.Fatalf("the answer went to %v, want only the routine's thread %s", map[string]bool{"routine": answer(child), "Asker's chat": answer(asker.ID)}, child)
	}
	// The routine's thread answers Helper as a bot, not as a routine report.
	jaz.release()
	select {
	case reply := <-replies:
		if !strings.HasPrefix(reply, "[reply from") || !strings.Contains(reply, "Thanks, noted.") {
			t.Fatalf("Helper got %q, want the routine thread's reply", reply)
		}
	case <-jaz.ctx.Done():
		t.Fatal("the routine's thread never answered Helper")
	}
}
